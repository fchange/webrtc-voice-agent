package bot

import (
	"log/slog"
	"testing"
	"time"

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
