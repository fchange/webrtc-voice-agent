package openaicompat

import (
	"io"
	"strings"
	"testing"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

func TestReadStreamHandlesSSEChunks(t *testing.T) {
	provider := NewLLM(configForTest(), nil)
	body := strings.NewReader(strings.Join([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":\"\"}]}",
		"",
		"data: {\"choices\":[{\"delta\":{\"content\":\"。\"},\"finish_reason\":\"\"}]}",
		"",
		"data: [DONE]",
		"",
	}, "\n"))

	out := make(chan adapters.LLMEvent, 8)
	provider.readStream(io.NopCloser(body), out)

	var events []adapters.LLMEvent
	for event := range out {
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Text != "你好" || events[0].Final {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Text != "。" || events[1].Final {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if !events[2].Final {
		t.Fatalf("expected final event, got %+v", events[2])
	}
}

func configForTest() config.OpenAICompatibleLLMConfig {
	return config.OpenAICompatibleLLMConfig{
		BaseURL:   "https://example.com/v1/chat/completions",
		APIKey:    "test",
		Model:     "Qwen2-7B-Instruct",
		Timeout:   0,
		MaxTokens: 128,
	}
}
