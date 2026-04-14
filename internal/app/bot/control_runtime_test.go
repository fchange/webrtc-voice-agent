package bot

import (
	"log/slog"
	"testing"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	dcproto "github.com/fchange/webrtc-voice-agent/internal/protocol/datachannel"
	"github.com/fchange/webrtc-voice-agent/internal/session"
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

func TestControlRuntimeVADStartDuringResponseInterruptsAndStartsNextTurn(t *testing.T) {
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

	runtime := newControlRuntime(manager, slog.Default())
	interrupted := false
	runtime.setInterruptHandler(func(sessionID string) {
		if sessionID == "sess_1" {
			interrupted = true
		}
	})

	runtime.handleVADStart("sess_1")

	if !interrupted {
		t.Fatal("expected interrupt handler to be invoked")
	}

	snapshot := task.Snapshot()
	if snapshot.State != session.StateProcessing {
		t.Fatalf("expected processing after barge-in vad start, got %s", snapshot.State)
	}
	if snapshot.CurrentTurn != 2 {
		t.Fatalf("expected next turn 2 after barge-in, got %d", snapshot.CurrentTurn)
	}

	pending := runtime.pending["sess_1"]
	if len(pending) != 4 {
		t.Fatalf("expected 4 pending events, got %d", len(pending))
	}
	if pending[0].Type != dcproto.TypeBotSpeakingStop {
		t.Fatalf("expected first event bot.speaking.stopped, got %s", pending[0].Type)
	}
	if pending[1].Type != dcproto.TypeTurnInterrupt {
		t.Fatalf("expected second event turn.interrupt, got %s", pending[1].Type)
	}
	if pending[2].Type != dcproto.TypeTurnStarted {
		t.Fatalf("expected third event turn.started, got %s", pending[2].Type)
	}
	if pending[3].Type != dcproto.TypeVADStarted {
		t.Fatalf("expected fourth event vad.started, got %s", pending[3].Type)
	}
	if pending[1].TurnID != 1 {
		t.Fatalf("expected interrupt event on turn 1, got %d", pending[1].TurnID)
	}
	if pending[2].TurnID != 2 || pending[3].TurnID != 2 {
		t.Fatalf("expected new turn events on turn 2, got started=%d vad=%d", pending[2].TurnID, pending[3].TurnID)
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

func TestControlRuntimeReadyHandlerCanBeRegistered(t *testing.T) {
	runtime := newControlRuntime(session.NewManager(time.Minute), slog.Default())
	called := false
	runtime.setReadyHandler(func(sessionID string) {
		if sessionID == "sess_1" {
			called = true
		}
	})

	runtime.mu.Lock()
	handler := runtime.onReady
	runtime.mu.Unlock()
	if handler == nil {
		t.Fatal("expected ready handler to be set")
	}

	handler("sess_1")
	if !called {
		t.Fatal("expected ready handler to be invoked")
	}
}

func TestControlRuntimeEmitsSessionEnding(t *testing.T) {
	manager := session.NewManager(time.Minute)
	runtime := newControlRuntime(manager, slog.Default())

	runtime.emitSessionEnding("sess_1", "本次预订已完成，通话即将结束。")

	pending := runtime.pending["sess_1"]
	if len(pending) != 1 {
		t.Fatalf("expected one pending event, got %d", len(pending))
	}
	if pending[0].Type != dcproto.TypeSessionEnding {
		t.Fatalf("expected session.ending, got %s", pending[0].Type)
	}
}
