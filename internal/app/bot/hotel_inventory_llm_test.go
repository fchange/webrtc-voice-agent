package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	"github.com/fchange/webrtc-voice-agent/internal/hotel"
)

type capturingLLM struct {
	req adapters.CompletionRequest
}

func (l *capturingLLM) Name() string {
	return "capturing-llm"
}

func (l *capturingLLM) Complete(_ context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	l.req = req
	out := make(chan adapters.LLMEvent, 1)
	out <- adapters.LLMEvent{Final: true}
	close(out)
	return out, nil
}

func TestHotelInventoryLLMInjectsCurrentRoomInventory(t *testing.T) {
	base := &capturingLLM{}
	wrapped := newHotelInventoryLLM(base, hotel.NewStore())

	events, err := wrapped.Complete(context.Background(), adapters.CompletionRequest{
		SessionID: "sess_1",
		TurnID:    1,
		Text:      "家庭套房还有吗",
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	for range events {
	}

	if !strings.Contains(base.req.Text, "name=家庭套房") {
		t.Fatalf("expected family suite in prompt, got %q", base.req.Text)
	}
	if !strings.Contains(base.req.Text, "available_count=1") {
		t.Fatalf("expected current availability in prompt, got %q", base.req.Text)
	}
	if !strings.Contains(base.req.Text, "家庭套房还有吗") {
		t.Fatalf("expected original transcript in prompt, got %q", base.req.Text)
	}
}
