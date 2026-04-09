package bot

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type failingASRProvider struct {
	err error
}

func (f failingASRProvider) Name() string {
	return "failing-asr"
}

func (f failingASRProvider) Transcribe(context.Context, <-chan adapters.AudioChunk) (<-chan adapters.ASREvent, error) {
	return nil, f.err
}

type capturingEmitter struct {
	sessionID string
	turnID    int64
	message   string
	events    []adapters.ASREvent
}

func (c *capturingEmitter) emitASREvent(_ string, _ int64, event adapters.ASREvent) {
	c.events = append(c.events, event)
}

func (c *capturingEmitter) emitError(sessionID string, turnID int64, message string) {
	c.sessionID = sessionID
	c.turnID = turnID
	c.message = message
}

func TestASRRuntimeEmitsControlErrorWhenProviderStartFails(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}
	if turnID, created, err := task.EnsureTurn(); err != nil || !created || turnID != 1 {
		t.Fatalf("ensure turn failed: turn=%d created=%v err=%v", turnID, created, err)
	}

	emitter := &capturingEmitter{}
	runtime := newASRRuntime(
		"sess_1",
		manager,
		failingASRProvider{err: errors.New("dial denied")},
		emitter,
		nil,
		16000,
		slog.Default(),
	)

	runtime.HandleSpeechStart()

	if emitter.sessionID != "sess_1" {
		t.Fatalf("expected session sess_1, got %q", emitter.sessionID)
	}
	if emitter.turnID != 1 {
		t.Fatalf("expected turn 1, got %d", emitter.turnID)
	}
	if emitter.message == "" {
		t.Fatal("expected error message to be emitted")
	}
}

func TestShouldPromoteStablePartial(t *testing.T) {
	now := time.Now().UTC()

	if shouldPromoteStablePartial("刚才已经把 a 搞定了。", 4, now.Add(-2*time.Second), now) == false {
		t.Fatal("expected punctuated stable partial to be promoted")
	}
	if shouldPromoteStablePartial("刚才已经把 a 搞定了", 4, now.Add(-2*time.Second), now) {
		t.Fatal("expected non-punctuated partial to remain partial")
	}
	if shouldPromoteStablePartial("刚才已经把 a 搞定了。", 2, now.Add(-2*time.Second), now) {
		t.Fatal("expected low repeat count not to promote")
	}
	if shouldPromoteStablePartial("刚才已经把 a 搞定了。", 4, now.Add(-300*time.Millisecond), now) {
		t.Fatal("expected short stability window not to promote")
	}
}

func TestASRRuntimePromotesTrailingPartialToFinalWhenProviderEndsWithoutFinal(t *testing.T) {
	emitter := &capturingEmitter{}
	runtime := newASRRuntime(
		"sess_1",
		session.NewManager(time.Minute),
		failingASRProvider{},
		emitter,
		nil,
		16000,
		slog.Default(),
	)

	events := make(chan adapters.ASREvent, 2)
	events <- adapters.ASREvent{Text: "早上好", Final: false}
	close(events)

	runtime.forwardEvents(1, make(chan adapters.AudioChunk), func() {}, events)

	if len(emitter.events) != 2 {
		t.Fatalf("expected partial plus promoted final, got %d events", len(emitter.events))
	}
	if emitter.events[0].Final {
		t.Fatalf("expected first event to remain partial, got %+v", emitter.events[0])
	}
	if !emitter.events[1].Final || emitter.events[1].Text != "早上好" {
		t.Fatalf("expected promoted final event, got %+v", emitter.events[1])
	}
}
