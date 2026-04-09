package volc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

func TestASRReady(t *testing.T) {
	provider := NewASR(config.VolcASRConfig{
		WSURL:       "wss://example.com/asr",
		AppID:       "app-id",
		AccessToken: "token",
		ResourceID:  "resource-id",
	}, slog.Default())

	if !provider.Ready() {
		t.Fatal("expected provider to be ready")
	}
}

func TestASRTranscribeHandshakeErrorIncludesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"requested resource not granted"}`)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	provider := NewASR(config.VolcASRConfig{
		WSURL:       wsURL,
		AppID:       "app-id",
		AccessToken: "token",
		ResourceID:  "resource-id",
	}, slog.Default())
	provider.dialer = websocket.DefaultDialer

	_, err := provider.Transcribe(context.Background(), make(chan adapters.AudioChunk))
	if err == nil {
		t.Fatal("expected handshake error")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("expected status in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "requested resource not granted") {
		t.Fatalf("expected response body in error, got %v", err)
	}
}

func TestASRDefaultOfficialEndpoint(t *testing.T) {
	t.Setenv("ASR_VOLC_WS_URL", "")
	cfg := config.LoadBotConfig()
	if cfg.ASR.VOLC.WSURL != "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async" {
		t.Fatalf("expected official volc asr websocket endpoint, got %q", cfg.ASR.VOLC.WSURL)
	}
}

func TestASRRequestConfigForOptimizedBidirectionalMode(t *testing.T) {
	provider := NewASR(config.VolcASRConfig{
		WSURL:       "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
		AppID:       "app-id",
		AccessToken: "token",
		ResourceID:  "resource-id",
		Language:    "zh-CN",
		SampleRate:  16000,
	}, slog.Default())

	audio := provider.audioConfig()
	if _, ok := audio["language"]; ok {
		t.Fatal("expected async mode to omit language in audio config")
	}

	request := provider.requestConfig()
	if got := request["show_utterances"]; got != true {
		t.Fatalf("expected show_utterances=true, got %#v", got)
	}
	if got := request["result_type"]; got != "single" {
		t.Fatalf("expected result_type=single, got %#v", got)
	}
}

func TestASREventsFromPayloadUsesDefiniteUtteranceForAsyncMode(t *testing.T) {
	provider := NewASR(config.VolcASRConfig{
		WSURL:       "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async",
		AppID:       "app-id",
		AccessToken: "token",
		ResourceID:  "resource-id",
	}, slog.Default())

	events := provider.eventsFromPayload(parsedPacket{}, volcASRPayload{
		Result: volcASRResult{
			Text: "ignored full text",
			Utterances: []volcASRUtterance{
				{Text: "你好，能听到吗？", Definite: true},
			},
		},
	})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Text != "你好，能听到吗？" || !events[0].Final {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}
