package xfyun

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

type TTS struct {
	cfg    config.XFYUNTTSConfig
	dialer *websocket.Dialer
	clock  func() time.Time
	logger *slog.Logger
}

func NewTTS(cfg config.XFYUNTTSConfig, logger *slog.Logger) *TTS {
	if logger == nil {
		logger = slog.Default()
	}
	return &TTS{
		cfg:    cfg,
		dialer: websocket.DefaultDialer,
		clock:  time.Now,
		logger: logger.With("provider", "xfyun-tts"),
	}
}

func (t *TTS) Name() string {
	return "xfyun-tts"
}

func (t *TTS) Ready() bool {
	return t.cfg.WSURL != "" && t.cfg.AppID != "" && t.cfg.APIKey != "" && t.cfg.APISecret != ""
}

func (t *TTS) Synthesize(ctx context.Context, req adapters.SynthesisRequest) (<-chan adapters.TTSEvent, error) {
	if !t.Ready() {
		return nil, fmt.Errorf("xfyun tts credentials are incomplete")
	}

	authURL, err := BuildAuthURL(t.cfg.WSURL, t.cfg.Host, t.cfg.RequestPath, t.cfg.APIKey, t.cfg.APISecret, t.clock().UTC())
	if err != nil {
		return nil, err
	}

	conn, resp, err := t.dialer.DialContext(ctx, authURL, http.Header{})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial xfyun tts: %w (status=%s)", err, resp.Status)
		}
		return nil, fmt.Errorf("dial xfyun tts: %w", err)
	}

	request := ttsRequestEnvelope{
		Common: ttsRequestCommon{
			AppID: t.cfg.AppID,
		},
		Business: ttsRequestBusiness{
			AudioEncoding: t.cfg.AudioEncoding,
			AudioFormat:   t.cfg.AudioFormat,
			Voice:         t.cfg.Voice,
			TextEncoding:  t.cfg.TextEncoding,
			Speed:         t.cfg.Speed,
			Volume:        t.cfg.Volume,
			Pitch:         t.cfg.Pitch,
			Background:    t.cfg.Background,
		},
		Data: ttsRequestData{
			Status: 2,
			Text:   base64.StdEncoding.EncodeToString([]byte(req.Text)),
		},
	}
	if err := conn.WriteJSON(request); err != nil {
		_ = conn.Close()
		return nil, err
	}

	t.logger.Info(
		"xfyun tts synthesis started",
		"session_id", req.SessionID,
		"turn_id", req.TurnID,
		"text", req.Text,
		"text_len", len(req.Text),
		"voice", t.cfg.Voice,
		"audio_format", t.cfg.AudioFormat,
		"audio_encoding", t.cfg.AudioEncoding,
	)

	out := make(chan adapters.TTSEvent, 32)
	go t.readSynthesis(ctx, conn, out, req)
	return out, nil
}

func (t *TTS) readSynthesis(ctx context.Context, conn *websocket.Conn, out chan<- adapters.TTSEvent, req adapters.SynthesisRequest) {
	defer close(out)
	defer conn.Close()

	chunks := 0
	bytes := 0
	for {
		select {
		case <-ctx.Done():
			t.logger.Info("xfyun tts synthesis cancelled", "session_id", req.SessionID, "turn_id", req.TurnID, "err", ctx.Err(), "chunks", chunks, "bytes", bytes)
			return
		default:
		}

		var response ttsResponseEnvelope
		if err := conn.ReadJSON(&response); err != nil {
			t.logger.Info("xfyun tts stream closed", "session_id", req.SessionID, "turn_id", req.TurnID, "err", err, "chunks", chunks, "bytes", bytes)
			return
		}
		if response.Code != 0 {
			t.logger.Error("xfyun tts response error", "session_id", req.SessionID, "turn_id", req.TurnID, "sid", response.Sid, "code", response.Code, "message", response.Message)
			return
		}

		audio, err := base64.StdEncoding.DecodeString(response.Data.Audio)
		if err != nil {
			t.logger.Error("decode xfyun tts audio failed", "session_id", req.SessionID, "turn_id", req.TurnID, "sid", response.Sid, "err", err)
			return
		}

		chunks++
		bytes += len(audio)
		t.logger.Info(
			"xfyun tts chunk received",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"sid", response.Sid,
			"status", response.Data.Status,
			"audio_bytes", len(audio),
			"ced", response.Data.Ced,
			"chunks", chunks,
			"bytes", bytes,
		)
		out <- adapters.TTSEvent{
			Chunk: adapters.AudioChunk{
				PCM: audio,
			},
			Final: response.Data.Status == 2,
		}

		if response.Data.Status == 2 {
			t.logger.Info("xfyun tts synthesis completed", "session_id", req.SessionID, "turn_id", req.TurnID, "sid", response.Sid, "chunks", chunks, "bytes", bytes)
			return
		}
	}
}

type ttsRequestEnvelope struct {
	Common   ttsRequestCommon   `json:"common"`
	Business ttsRequestBusiness `json:"business"`
	Data     ttsRequestData     `json:"data"`
}

type ttsRequestCommon struct {
	AppID string `json:"app_id"`
}

type ttsRequestBusiness struct {
	AudioEncoding string `json:"aue"`
	AudioFormat   string `json:"auf"`
	Voice         string `json:"vcn"`
	TextEncoding  string `json:"tte"`
	Speed         int    `json:"speed,omitempty"`
	Volume        int    `json:"volume,omitempty"`
	Pitch         int    `json:"pitch,omitempty"`
	Background    int    `json:"bgs,omitempty"`
}

type ttsRequestData struct {
	Status int    `json:"status"`
	Text   string `json:"text"`
}

type ttsResponseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Sid     string          `json:"sid"`
	Data    ttsResponseData `json:"data"`
}

type ttsResponseData struct {
	Audio  string `json:"audio"`
	Status int    `json:"status"`
	Ced    string `json:"ced,omitempty"`
}

func MarshalTTSTextBase64(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func DecodeTTSAudioBase64(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encoded)
}

func DecodeTTSResponseText(data []byte) (ttsResponseEnvelope, error) {
	var response ttsResponseEnvelope
	err := json.Unmarshal(data, &response)
	return response, err
}
