package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/hotel"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/observability"
	protoerrors "github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/errors"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/signaling"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type Dependencies struct {
	Manager   *session.Manager
	Metrics   *observability.Metrics
	Providers adapters.ProviderBundle
	Hotel     *hotel.Store
}

type Server struct {
	cfg    config.BotConfig
	logger *slog.Logger
	deps   Dependencies
	rtc    *rtcManager
}

var errSignalSessionClosed = errors.New("signal session closed")

func NewServer(cfg config.BotConfig, logger *slog.Logger, deps Dependencies) *Server {
	if deps.Hotel == nil {
		deps.Hotel = hotel.NewStore()
	}
	control := newControlRuntime(deps.Manager, logger)

	asrSampleRate := uint32(cfg.VAD.SampleRate)
	switch cfg.ASR.Provider {
	case "xfyun-spark-iat":
		if cfg.ASR.XFYUN.SampleRate > 0 {
			asrSampleRate = uint32(cfg.ASR.XFYUN.SampleRate)
		}
	case "volc-doubao-asr":
		if cfg.ASR.VOLC.SampleRate > 0 {
			asrSampleRate = uint32(cfg.ASR.VOLC.SampleRate)
		}
	}
	rtc := newRTCManager(
		cfg.STUNURL,
		logger,
		deps.Manager,
		control,
		deps.Providers.ASR,
		deps.Providers.LLM,
		deps.Providers.TTS,
		asrSampleRate,
		cfg.LLM.Segmenter,
		cfg.TTS.XFYUN,
	)
	control.setInterruptHandler(rtc.interruptResponse)
	return &Server{
		cfg:    cfg,
		logger: logger,
		deps:   deps,
		rtc:    rtc,
	}
}

func (s *Server) Run() error {
	s.logger.Info("bot server starting", "addr", s.cfg.Addr, "stun_url", s.cfg.STUNURL)
	return http.ListenAndServe(s.cfg.Addr, s.handler())
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/bot/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/bot/sessions/{sessionID}", s.handleGetSession)
	mux.HandleFunc("POST /v1/bot/sessions/{sessionID}/turns", s.handleStartTurn)
	mux.HandleFunc("POST /v1/bot/sessions/{sessionID}/interrupt", s.handleInterrupt)
	mux.HandleFunc("POST /v1/bot/sessions/{sessionID}/end", s.handleEndSession)
	mux.HandleFunc("GET /internal/room-types", s.handleListRoomTypes)
	mux.HandleFunc("GET /internal/reservations", s.handleListReservations)
	mux.HandleFunc("POST /internal/reservations", s.handleCreateReservation)
	return s.withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":         "bot",
		"status":          "ok",
		"active_sessions": s.deps.Manager.Count(),
		"metrics":         s.deps.Metrics.Snapshot(),
		"hotel": map[string]any{
			"room_types":    len(s.deps.Hotel.ListRoomTypes()),
			"reservations":  len(s.deps.Hotel.ListReservations(0)),
			"service_ready": s.deps.Hotel != nil,
		},
		"providers": map[string]string{
			"asr": s.deps.Providers.ASR.Name(),
			"llm": s.deps.Providers.LLM.Name(),
			"tts": s.deps.Providers.TTS.Name(),
		},
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%d", time.Now().UTC().UnixNano())
	}

	task := s.deps.Manager.Create(sessionID)
	_ = task.BeginNegotiation()
	_ = task.MarkActive()
	s.deps.Metrics.Inc("session_created_total")
	go s.attachToSignal(sessionID)

	s.logger.Info("bot session created", "session_id", sessionID)
	writeJSON(w, http.StatusCreated, task.Snapshot())
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	task, ok := s.deps.Manager.Get(r.PathValue("sessionID"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "session not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, task.Snapshot())
}

func (s *Server) handleStartTurn(w http.ResponseWriter, r *http.Request) {
	task, ok := s.deps.Manager.Get(r.PathValue("sessionID"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
		return
	}

	turnID, err := task.StartTurn()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}

	_ = task.StartResponse(turnID)
	s.deps.Metrics.Inc("turn_started_total")
	writeJSON(w, http.StatusAccepted, map[string]any{
		"session_id": r.PathValue("sessionID"),
		"turn_id":    turnID,
		"state":      task.Snapshot().State,
	})
}

func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	task, ok := s.deps.Manager.Get(r.PathValue("sessionID"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "user_barge_in"
	}

	result, err := task.Interrupt(req.Reason)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}

	s.deps.Metrics.Inc("turn_interrupt_total")
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request) {
	if !s.deps.Manager.End(r.PathValue("sessionID")) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
		return
	}

	s.rtc.Close(r.PathValue("sessionID"))
	s.deps.Metrics.Inc("session_ended_total")
	writeJSON(w, http.StatusOK, map[string]any{"status": "closed"})
}

func (s *Server) handleListRoomTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"room_types": s.deps.Hotel.ListRoomTypes(),
	})
}

func (s *Server) handleListReservations(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reservations": s.deps.Hotel.ListReservations(limit),
	})
}

func (s *Server) handleCreateReservation(w http.ResponseWriter, r *http.Request) {
	var req hotel.CreateReservationInput
	_ = json.NewDecoder(r.Body).Decode(&req)

	result := s.deps.Hotel.CreateReservation(req)
	statusCode := http.StatusCreated
	switch result.Status {
	case hotel.ReservationStatusInvalidInput:
		statusCode = http.StatusBadRequest
	case hotel.ReservationStatusSoldOut:
		statusCode = http.StatusConflict
	case hotel.ReservationStatusFailed:
		statusCode = http.StatusInternalServerError
	}

	writeJSON(w, statusCode, result)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func mustMarshal(payload any) json.RawMessage {
	if payload == nil {
		return nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}

func (s *Server) attachToSignal(sessionID string) {
	if s.cfg.SignalWSURL == "" {
		return
	}

	wsURL, err := url.Parse(s.cfg.SignalWSURL)
	if err != nil {
		s.logger.Error("invalid signal ws url", "err", err, "session_id", sessionID)
		return
	}

	query := wsURL.Query()
	query.Set("session_id", sessionID)
	query.Set("role", "bot")
	query.Set("access_token", s.cfg.SignalToken)
	wsURL.RawQuery = query.Encode()

	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL.String(), nil)
	if err != nil {
		s.logger.Error("attach to signal failed", "err", err, "session_id", sessionID)
		return
	}
	defer conn.Close()

	writer := &lockedSignalWriter{conn: conn}
	s.logger.Info("bot attached to signal", "session_id", sessionID)

	for {
		var envelope signaling.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			s.logger.Info("bot signal connection closed", "session_id", sessionID, "err", err)
			return
		}

		if err := s.handleSignalMessage(writer, envelope); err != nil {
			if errors.Is(err, errSignalSessionClosed) {
				s.logger.Info("signal requested session close", "session_id", sessionID)
				return
			}
			s.logger.Error("handle signal message failed", "session_id", sessionID, "err", err)
			_ = writer.WriteEnvelope(signaling.Envelope{
				Version:   signaling.Version,
				Type:      signaling.TypeSessionError,
				SessionID: sessionID,
				Payload: mustMarshal(signaling.ErrorPayload{
					Code:    protoerrors.CodeInvalidState,
					Message: err.Error(),
				}),
			})
		}
	}
}

func (s *Server) handleSignalMessage(writer signalWriter, envelope signaling.Envelope) error {
	switch envelope.Type {
	case signaling.TypeSessionAttached:
		return nil
	case signaling.TypeSessionOffer:
		var payload signaling.SDPPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf("decode offer: %w", err)
		}
		if payload.Type != "offer" {
			return fmt.Errorf("expected offer payload, got %q", payload.Type)
		}
		return s.rtc.HandleOffer(envelope.SessionID, payload, writer)
	case signaling.TypeSessionICE:
		var payload signaling.ICECandidatePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf("decode ice candidate: %w", err)
		}
		return s.rtc.HandleICECandidate(envelope.SessionID, payload)
	case signaling.TypeSessionClose:
		s.rtc.Close(envelope.SessionID)
		_ = s.deps.Manager.End(envelope.SessionID)
		return errSignalSessionClosed
	default:
		return nil
	}
}

type lockedSignalWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *lockedSignalWriter) WriteEnvelope(envelope signaling.Envelope) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(envelope)
}
