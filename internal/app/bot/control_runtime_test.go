package bot

import (
	"log/slog"
	"testing"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	dcproto "github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/datachannel"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

func TestControlRuntimeInterruptHintPromotesInterrupt(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	turnID, created, err := task.EnsureTurn()
	if err != nil {
		t.Fatalf("ensure turn: %v", err)
	}
	if !created || turnID != 1 {
		t.Fatalf("expected turn 1 created, got created=%v turn=%d", created, turnID)
	}
	if err := task.StartResponse(turnID); err != nil {
		t.Fatalf("start response: %v", err)
	}

	result, err := task.Interrupt("user_barge_in")
	if err != nil {
		t.Fatalf("interrupt: %v", err)
	}

	if result.InterruptedTurnID != 1 {
		t.Fatalf("expected interrupted turn 1, got %d", result.InterruptedTurnID)
	}
	if result.NextTurnID != 2 {
		t.Fatalf("expected next turn 2, got %d", result.NextTurnID)
	}
	if got := task.Snapshot().State; got != session.StateActive {
		t.Fatalf("expected task active after interrupt, got %s", got)
	}
}

func TestControlRuntimeVADLifecycleLeavesTurnOpenForResponsePipeline(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	runtime := newControlRuntime(manager, slog.Default())
	runtime.handleVADStart("sess_1")

	snapshot := task.Snapshot()
	if snapshot.CurrentTurn != 1 || snapshot.State != session.StateProcessing {
		t.Fatalf("expected processing turn 1 after vad start, got turn=%d state=%s", snapshot.CurrentTurn, snapshot.State)
	}

	runtime.handleVADEnd("sess_1")
	runtime.handleEndOfUtterance("sess_1")

	snapshot = task.Snapshot()
	if snapshot.State != session.StateProcessing {
		t.Fatalf("expected processing after end of utterance, got %s", snapshot.State)
	}
}

func TestControlRuntimeVADStartAfterInterruptUsesExpectedNextTurn(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	turnID, created, err := task.EnsureTurn()
	if err != nil || !created {
		t.Fatalf("ensure turn failed: created=%v err=%v", created, err)
	}
	if err := task.StartResponse(turnID); err != nil {
		t.Fatalf("start response: %v", err)
	}
	if _, err := task.Interrupt("user_barge_in"); err != nil {
		t.Fatalf("interrupt: %v", err)
	}

	runtime := newControlRuntime(manager, slog.Default())
	runtime.handleVADStart("sess_1")

	snapshot := task.Snapshot()
	if snapshot.State != session.StateProcessing {
		t.Fatalf("expected processing after next vad start, got %s", snapshot.State)
	}
	if snapshot.CurrentTurn != 2 {
		t.Fatalf("expected next turn to be 2 after interrupt, got %d", snapshot.CurrentTurn)
	}
}

func TestControlRuntimeBuffersASREventUntilDataChannelOpens(t *testing.T) {
	manager := session.NewManager(time.Minute)
	runtime := newControlRuntime(manager, slog.Default())

	runtime.emitASREvent("sess_1", 1, adapters.ASREvent{
		Text:  "你好",
		Final: true,
	})

	pending := runtime.pending["sess_1"]
	if len(pending) != 1 {
		t.Fatalf("expected one pending event, got %d", len(pending))
	}
	if pending[0].Type != dcproto.TypeASRFinal {
		t.Fatalf("expected pending asr.final, got %s", pending[0].Type)
	}
	payload, ok := pending[0].Payload.(dcproto.TranscriptPayload)
	if !ok {
		t.Fatalf("expected transcript payload, got %T", pending[0].Payload)
	}
	if payload.Text != "你好" || !payload.Final {
		t.Fatalf("unexpected transcript payload: %+v", payload)
	}
}

func TestAppendPendingEnvelopeKeepsNewestEvents(t *testing.T) {
	var pending []dcproto.Envelope
	for i := 0; i < maxPendingControlEvents+3; i++ {
		pending = appendPendingEnvelope(pending, dcproto.Envelope{
			Version: dcproto.Version,
			Type:    dcproto.TypeLLMPartial,
			TurnID:  int64(i),
		})
	}

	if len(pending) != maxPendingControlEvents {
		t.Fatalf("expected capped pending events, got %d", len(pending))
	}
	if pending[0].TurnID != 3 {
		t.Fatalf("expected oldest retained event to be turn 3, got %d", pending[0].TurnID)
	}
}
