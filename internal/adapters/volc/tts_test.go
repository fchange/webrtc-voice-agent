package volc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	"github.com/fchange/webrtc-voice-agent/internal/config"
)

func TestTTSReady(t *testing.T) {
	complete := testVolcTTSConfig("https://example.com/tts")

	tests := []struct {
		name string
		cfg  config.VolcTTSConfig
		want bool
	}{
		{name: "complete", cfg: complete, want: true},
		{name: "missing base url", cfg: func() config.VolcTTSConfig {
			cfg := complete
			cfg.BaseURL = ""
			return cfg
		}(), want: false},
		{name: "missing access token", cfg: func() config.VolcTTSConfig {
			cfg := complete
			cfg.AccessToken = ""
			return cfg
		}(), want: false},
		{name: "missing app id", cfg: func() config.VolcTTSConfig {
			cfg := complete
			cfg.AppID = ""
			return cfg
		}(), want: false},
		{name: "missing resource id", cfg: func() config.VolcTTSConfig {
			cfg := complete
			cfg.ResourceID = ""
			return cfg
		}(), want: true},
		{name: "missing cluster", cfg: func() config.VolcTTSConfig {
			cfg := complete
			cfg.Cluster = ""
			return cfg
		}(), want: false},
		{name: "missing voice type", cfg: func() config.VolcTTSConfig {
			cfg := complete
			cfg.VoiceType = ""
			return cfg
		}(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewTTS(tt.cfg, discardLogger())
			if got := provider.Ready(); got != tt.want {
				t.Fatalf("Ready() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTTSSynthesizePostsExpectedRequestAndEmitsAudio(t *testing.T) {
	audio := []byte("pcm audio bytes")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected application/json content type, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer;token" {
			t.Fatalf("expected authorization header, got %q", got)
		}
		if got := r.Header.Get("Resource-Id"); got != "" {
			t.Fatalf("expected no Resource-Id header for v1 tts, got %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "" {
			t.Fatalf("expected no X-Api-Resource-Id header for v1 tts, got %q", got)
		}

		var body volcTTSRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.App.AppID != "app-id" || body.App.Token != "token" || body.App.Cluster != "volcano_tts" {
			t.Fatalf("unexpected app config: %+v", body.App)
		}
		if !strings.HasPrefix(body.User.UID, "uid_") {
			t.Fatalf("expected generated uid prefix, got %q", body.User.UID)
		}
		if body.Audio.VoiceType != "BV007_streaming" {
			t.Fatalf("expected voice type, got %q", body.Audio.VoiceType)
		}
		if body.Audio.Encoding != "pcm" || body.Audio.Rate != 16000 {
			t.Fatalf("unexpected audio config: %+v", body.Audio)
		}
		if body.Audio.SpeedRatio != 1.1 || body.Audio.VolumeRatio != 1.2 || body.Audio.PitchRatio != 0.9 {
			t.Fatalf("unexpected audio ratios: %+v", body.Audio)
		}
		if body.Request.ReqID == "" {
			t.Fatal("expected generated reqid")
		}
		if body.Request.Text != "你好，火山 TTS" || body.Request.TextType != "plain" || body.Request.Operation != "query" {
			t.Fatalf("unexpected request config: %+v", body.Request)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(volcTTSResponse{
			ReqID:   "response-req-id",
			Code:    3000,
			Message: "Success",
			Data:    base64.StdEncoding.EncodeToString(audio),
		})
	}))
	defer server.Close()

	provider := NewTTS(testVolcTTSConfig(server.URL), discardLogger())
	events, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{
		SessionID: "session-1",
		TurnID:    7,
		Text:      "你好，火山 TTS",
	})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}

	event, ok := <-events
	if !ok {
		t.Fatal("expected one tts event")
	}
	if !event.Final {
		t.Fatalf("expected final event, got %+v", event)
	}
	if string(event.Chunk.PCM) != string(audio) {
		t.Fatalf("expected audio %q, got %q", string(audio), string(event.Chunk.PCM))
	}
	if _, ok := <-events; ok {
		t.Fatal("expected tts event channel to be closed")
	}
}

func TestTTSSynthesizeRequiresReadyConfig(t *testing.T) {
	provider := NewTTS(config.VolcTTSConfig{}, discardLogger())

	_, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected incomplete credentials error")
	}
	if !strings.Contains(err.Error(), "credentials are incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTTSSynthesizeDoesNotRequireResourceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Resource-Id"); got != "" {
			t.Fatalf("expected no Resource-Id header for v1 tts, got %q", got)
		}
		if got := r.Header.Get("X-Api-Resource-Id"); got != "" {
			t.Fatalf("expected no X-Api-Resource-Id header for v1 tts, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(volcTTSResponse{
			ReqID:   "response-req-id",
			Code:    3000,
			Message: "Success",
			Data:    base64.StdEncoding.EncodeToString([]byte("ok")),
		})
	}))
	defer server.Close()

	cfg := testVolcTTSConfig(server.URL)
	cfg.ResourceID = ""

	provider := NewTTS(cfg, discardLogger())
	events, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("synthesize without resource id: %v", err)
	}
	if event, ok := <-events; !ok || !event.Final {
		t.Fatalf("expected final event, got ok=%v event=%+v", ok, event)
	}
	if _, ok := <-events; ok {
		t.Fatal("expected tts event channel to be closed")
	}
}

func TestTTSSynthesizeHTTPErrorIncludesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"requested resource not granted"}`)
	}))
	defer server.Close()

	provider := NewTTS(testVolcTTSConfig(server.URL), discardLogger())
	_, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected http error")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("expected status in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "requested resource not granted") {
		t.Fatalf("expected response body in error, got %v", err)
	}
}

func TestTTSSynthesizeReturnsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(volcTTSResponse{
			ReqID:   "response-req-id",
			Code:    4001,
			Message: "invalid voice type",
		})
	}))
	defer server.Close()

	provider := NewTTS(testVolcTTSConfig(server.URL), discardLogger())
	_, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected business error")
	}
	if !strings.Contains(err.Error(), "code=4001") || !strings.Contains(err.Error(), "invalid voice type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTTSSynthesizeReturnsBase64DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(volcTTSResponse{
			ReqID:   "response-req-id",
			Code:    3000,
			Message: "Success",
			Data:    "%%%not-base64%%%",
		})
	}))
	defer server.Close()

	provider := NewTTS(testVolcTTSConfig(server.URL), discardLogger())
	_, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected base64 decode error")
	}
	if !strings.Contains(err.Error(), "decode volc tts audio base64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTTSDefaultOfficialEndpoint(t *testing.T) {
	t.Setenv("TTS_VOLC_BASE_URL", "")

	cfg := config.LoadBotConfig()
	if cfg.TTS.VOLC.BaseURL != "https://openspeech.bytedance.com/api/v1/tts" {
		t.Fatalf("expected official volc tts endpoint, got %q", cfg.TTS.VOLC.BaseURL)
	}
}

func TestTTSSynthesizeWithEnvConfigWritesTempFile(t *testing.T) {
	cfg := config.LoadBotConfig().TTS.VOLC
	if cfg.BaseURL == "" || cfg.AccessToken == "" || cfg.AppID == "" || cfg.Cluster == "" || cfg.VoiceType == "" {
		t.Skip("skip live env tts test: incomplete TTS_VOLC_* config")
	}
	if isPlaceholderVolcValue(cfg.AccessToken) || isPlaceholderVolcValue(cfg.AppID) {
		t.Skip("skip live env tts test: placeholder TTS_VOLC_* credentials")
	}

	provider := NewTTS(cfg, discardLogger())
	events, err := provider.Synthesize(context.Background(), adapters.SynthesisRequest{
		SessionID: "env-live-test",
		TurnID:    1,
		Text:      "这是一个环境配置驱动的 TTS 联调用例。",
	})
	if err != nil {
		t.Fatalf("synthesize with env config: %v", err)
	}

	var audio []byte
	for event := range events {
		audio = append(audio, event.Chunk.PCM...)
	}
	if len(audio) == 0 {
		t.Fatal("expected synthesized audio bytes")
	}

	outputPath := filepath.Join(t.TempDir(), "volc-live-output.pcm")
	if err := os.WriteFile(outputPath, audio, 0o644); err != nil {
		t.Fatalf("write temp audio file: %v", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat temp audio file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected non-empty temp audio file at %s", outputPath)
	}
	t.Logf("wrote synthesized audio to %s (%d bytes)", outputPath, info.Size())
}

func testVolcTTSConfig(baseURL string) config.VolcTTSConfig {
	return config.VolcTTSConfig{
		BaseURL:     baseURL,
		AccessToken: "token",
		AppID:       "app-id",
		ResourceID:  "resource-id",
		Cluster:     "volcano_tts",
		VoiceType:   "BV007_streaming",
		Encoding:    "pcm",
		SampleRate:  16000,
		SpeedRatio:  1.1,
		VolumeRatio: 1.2,
		PitchRatio:  0.9,
		Timeout:     2 * time.Second,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func isPlaceholderVolcValue(value string) bool {
	return strings.HasPrefix(value, "your_")
}
