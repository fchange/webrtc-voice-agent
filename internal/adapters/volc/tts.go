package volc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

type TTS struct {
	cfg    config.VolcTTSConfig
	client *http.Client
	logger *slog.Logger
}

func NewTTS(cfg config.VolcTTSConfig, logger *slog.Logger) *TTS {
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &TTS{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
		logger: logger.With("provider", "volc-doubao-tts"),
	}
}

func (t *TTS) Name() string { return "volc-doubao-tts" }

func (t *TTS) Ready() bool {
	return t.cfg.BaseURL != "" && t.cfg.AccessToken != "" && t.cfg.AppID != "" && t.cfg.ResourceID != "" && t.cfg.Cluster != "" && t.cfg.VoiceType != ""
}

func (t *TTS) Synthesize(ctx context.Context, req adapters.SynthesisRequest) (<-chan adapters.TTSEvent, error) {
	if !t.Ready() {
		return nil, fmt.Errorf("volc tts credentials are incomplete")
	}

	reqID := uuid.NewString()
	requestBody, err := json.Marshal(volcTTSRequest{
		App: volcTTSApp{
			AppID:   t.cfg.AppID,
			Token:   t.cfg.AccessToken, // docs: any non-empty is ok, but using the same token helps debugging
			Cluster: t.cfg.Cluster,
		},
		User: volcTTSUser{
			UID: "uid_" + uuid.NewString(),
		},
		Audio: volcTTSAudio{
			VoiceType:   t.cfg.VoiceType,
			Encoding:    t.cfg.Encoding,
			Rate:        t.cfg.SampleRate,
			SpeedRatio:  t.cfg.SpeedRatio,
			VolumeRatio: t.cfg.VolumeRatio,
			PitchRatio:  t.cfg.PitchRatio,
		},
		Request: volcTTSReq{
			ReqID:     reqID,
			Text:      req.Text,
			TextType:  "plain",
			Operation: "query",
		},
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.BaseURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer;"+t.cfg.AccessToken)
	httpReq.Header.Set("Resource-Id", t.cfg.ResourceID)
	httpReq.Header.Set("X-Api-Resource-Id", t.cfg.ResourceID)
	t.logger.Info(
		"volc tts request started",
		"session_id", req.SessionID,
		"turn_id", req.TurnID,
		"req_id", reqID,
		"text_len", len(req.Text),
		"voice_type", t.cfg.VoiceType,
		"encoding", t.cfg.Encoding,
		"sample_rate", t.cfg.SampleRate,
		"resource_id", t.cfg.ResourceID,
		"cluster", t.cfg.Cluster,
	)

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		t.logger.Error(
			"volc tts http error",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"req_id", reqID,
			"status", resp.Status,
			"tt_logid", resp.Header.Get("X-Tt-Logid"),
			"body", strings.TrimSpace(string(data)),
		)
		return nil, fmt.Errorf("volc tts status=%s body=%s", resp.Status, strings.TrimSpace(string(data)))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return nil, err
	}

	var decoded volcTTSResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode volc tts response: %w", err)
	}
	if decoded.Code != 3000 {
		t.logger.Error(
			"volc tts business error",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"req_id", reqID,
			"resp_reqid", decoded.ReqID,
			"code", decoded.Code,
			"message", decoded.Message,
		)
		return nil, fmt.Errorf("volc tts code=%d message=%s", decoded.Code, decoded.Message)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(decoded.Data)
	if err != nil {
		return nil, fmt.Errorf("decode volc tts audio base64: %w", err)
	}

	t.logger.Info(
		"volc tts synthesized",
		"session_id", req.SessionID,
		"turn_id", req.TurnID,
		"req_id", reqID,
		"resp_reqid", decoded.ReqID,
		"text_len", len(req.Text),
		"bytes", len(audioBytes),
		"encoding", t.cfg.Encoding,
		"rate", t.cfg.SampleRate,
		"voice_type", t.cfg.VoiceType,
	)

	out := make(chan adapters.TTSEvent, 1)
	out <- adapters.TTSEvent{
		Chunk: adapters.AudioChunk{
			PCM: audioBytes,
		},
		Final: true,
	}
	close(out)
	return out, nil
}

type volcTTSRequest struct {
	App     volcTTSApp   `json:"app"`
	User    volcTTSUser  `json:"user"`
	Audio   volcTTSAudio `json:"audio"`
	Request volcTTSReq   `json:"request"`
}

type volcTTSApp struct {
	AppID   string `json:"appid"`
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

type volcTTSUser struct {
	UID string `json:"uid"`
}

type volcTTSAudio struct {
	VoiceType   string  `json:"voice_type"`
	Encoding    string  `json:"encoding"`
	Rate        int     `json:"rate,omitempty"`
	SpeedRatio  float64 `json:"speed_ratio,omitempty"`
	VolumeRatio float64 `json:"volume_ratio,omitempty"`
	PitchRatio  float64 `json:"pitch_ratio,omitempty"`
}

type volcTTSReq struct {
	ReqID     string `json:"reqid"`
	Text      string `json:"text"`
	TextType  string `json:"text_type"`
	Operation string `json:"operation"`
}

type volcTTSResponse struct {
	ReqID   string `json:"reqid"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}
