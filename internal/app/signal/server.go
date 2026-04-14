package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/config"
	protoerrors "github.com/fchange/webrtc-voice-agent/internal/protocol/errors"
	"github.com/fchange/webrtc-voice-agent/internal/protocol/signaling"
	"github.com/gorilla/websocket"
)

type Server struct {
	cfg      config.SignalConfig
	logger   *slog.Logger
	hub      *hub
	upgrader websocket.Upgrader
}

func NewServer(cfg config.SignalConfig, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
		hub:    newHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Run() error {
	s.logger.Info("signal server starting", "addr", s.cfg.Addr)
	return http.ListenAndServe(s.cfg.Addr, s.handler())
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "signal",
		"status":  "ok",
		"ws_url":  s.cfg.PublicWSURL,
		"bot_url": s.cfg.BotBaseURL,
	})
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /v1/sessions/{sessionID}", s.handleGetSession)
	mux.HandleFunc("GET /ws", s.handleWebSocket)
	return s.withCORS(mux)
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
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": map[string]any{
				"code":    "unauthorized",
				"message": "missing or invalid bearer token",
			},
		})
		return
	}

	var req signaling.CreateSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	sessionID := fmt.Sprintf("sess_%d", time.Now().UTC().UnixNano())
	s.hub.createSession(sessionID)

	if err := s.registerBotSession(r.Context(), sessionID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{
				"code":    "bot_unavailable",
				"message": err.Error(),
			},
		})
		return
	}

	resp := signaling.CreateSessionResponse{
		SessionID:      sessionID,
		SignalingWSURL: s.cfg.PublicWSURL,
		ICEUrls:        []string{"stun:stun.l.google.com:19302"},
		TokenHint:      "use ?access_token=<token>&role=client for WebSocket signaling",
	}

	s.logger.Info("session created", "session_id", sessionID, "client_id", req.ClientID)
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	createdAt, ok := s.hub.createdAt(sessionID)

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"code":    "session_not_found",
				"message": "session not found",
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"status":     "created",
		"created_at": createdAt,
		"next_step":  "connect websocket at /ws?session_id=<id>&role=client&access_token=<token>",
	})
}

func (s *Server) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if header == "" {
		return false
	}

	token := strings.TrimPrefix(header, "Bearer ")
	return token == s.cfg.DevToken
}

func (s *Server) wsAuthorized(r *http.Request) bool {
	return r.URL.Query().Get("access_token") == s.cfg.DevToken
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.wsAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	role := sessionRole(r.URL.Query().Get("role"))
	if sessionID == "" || (role != roleClient && role != roleBot) {
		http.Error(w, "missing session_id or invalid role", http.StatusBadRequest)
		return
	}
	if !s.hub.hasSession(sessionID) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	peer, queued, err := s.hub.attach(sessionID, role, conn)
	if err != nil {
		_ = conn.WriteJSON(signaling.Envelope{
			Version:   signaling.Version,
			Type:      signaling.TypeSessionError,
			SessionID: sessionID,
			Payload: mustMarshal(signaling.ErrorPayload{
				Code:    protoerrors.CodeConflict,
				Message: err.Error(),
			}),
		})
		_ = conn.Close()
		return
	}

	s.logger.Info("ws attached", "session_id", sessionID, "role", role)
	go s.writeLoop(sessionID, peer)
	peer.send <- signaling.Envelope{
		Version:   signaling.Version,
		Type:      signaling.TypeSessionAttached,
		SessionID: sessionID,
		Payload: mustMarshal(signaling.AttachPayload{
			Role: string(role),
		}),
	}
	for _, msg := range queued {
		peer.send <- msg
	}
	s.readLoop(sessionID, peer)
}

func (s *Server) readLoop(sessionID string, peer *peer) {
	defer func() {
		s.hub.detach(sessionID, peer.role)
		_ = peer.conn.Close()
		s.logger.Info("ws detached", "session_id", sessionID, "role", peer.role)
	}()

	for {
		var envelope signaling.Envelope
		if err := peer.conn.ReadJSON(&envelope); err != nil {
			return
		}
		if envelope.SessionID == "" {
			envelope.SessionID = sessionID
		}
		if envelope.SessionID != sessionID {
			peer.send <- signaling.Envelope{
				Version:   signaling.Version,
				Type:      signaling.TypeSessionError,
				SessionID: sessionID,
				Payload: mustMarshal(signaling.ErrorPayload{
					Code:    protoerrors.CodeInvalidMessage,
					Message: "session_id mismatch",
				}),
			}
			continue
		}
		if envelope.Version == "" {
			envelope.Version = signaling.Version
		}
		if err := envelope.Validate(); err != nil {
			peer.send <- signaling.Envelope{
				Version:   signaling.Version,
				Type:      signaling.TypeSessionError,
				SessionID: sessionID,
				Payload: mustMarshal(signaling.ErrorPayload{
					Code:    protoerrors.CodeInvalidMessage,
					Message: err.Error(),
				}),
			}
			continue
		}
		if err := s.hub.relay(sessionID, peer.role, envelope); err != nil {
			peer.send <- signaling.Envelope{
				Version:   signaling.Version,
				Type:      signaling.TypeSessionError,
				SessionID: sessionID,
				Payload: mustMarshal(signaling.ErrorPayload{
					Code:    protoerrors.CodeInternal,
					Message: err.Error(),
				}),
			}
		}
	}
}

func (s *Server) writeLoop(sessionID string, peer *peer) {
	for envelope := range peer.send {
		if err := peer.conn.WriteJSON(envelope); err != nil {
			return
		}
	}
}

func (s *Server) registerBotSession(ctx context.Context, sessionID string) error {
	body, err := json.Marshal(map[string]string{"session_id": sessionID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BotBaseURL+"/v1/bot/sessions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("register bot session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register bot session returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
