package bot

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type testLLM struct {
	mu       sync.Mutex
	events   []adapters.LLMEvent
	requests []adapters.CompletionRequest
}

func (t *testLLM) Name() string {
	return "test-llm"
}

func (t *testLLM) Complete(ctx context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	t.mu.Lock()
	t.requests = append(t.requests, req)
	t.mu.Unlock()

	out := make(chan adapters.LLMEvent, len(t.events))
	go func() {
		defer close(out)
		for _, event := range t.events {
			select {
			case <-ctx.Done():
				return
			case out <- event:
			}
		}
	}()
	return out, nil
}

type testTTS struct {
	mu    sync.Mutex
	texts []string
}

func (t *testTTS) Name() string {
	return "test-tts"
}

func (t *testTTS) Synthesize(ctx context.Context, req adapters.SynthesisRequest) (<-chan adapters.TTSEvent, error) {
	t.mu.Lock()
	t.texts = append(t.texts, req.Text)
	t.mu.Unlock()

	out := make(chan adapters.TTSEvent, 1)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case out <- adapters.TTSEvent{Chunk: adapters.AudioChunk{PCM: []byte("ok")}, Final: true}:
		}
	}()
	return out, nil
}

func TestResponseRuntimeSegmentsAtPunctuationAndCompletesTurn(t *testing.T) {
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

	tts := &testTTS{}
	llm := &testLLM{
		events: []adapters.LLMEvent{
			{Text: "你好，世界。再见"},
			{Final: true},
		},
	}
	runtime := newResponseRuntime(
		"sess_1",
		manager,
		llm,
		tts,
		newControlRuntime(manager, slog.Default()),
		config.LLMSegmenterConfig{Mode: "punctuation_boundary", Punctuation: "。！？；!?;"},
		nil,
		nil,
		slog.Default(),
	)

	runtime.HandleASREvent(turnID, adapters.ASREvent{Text: "用户输入", Final: true})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := task.Snapshot().State; got == session.StateActive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := task.Snapshot().State; got != session.StateActive {
		t.Fatalf("expected active after response pipeline, got %s", got)
	}

	tts.mu.Lock()
	defer tts.mu.Unlock()
	if len(tts.texts) != 2 {
		t.Fatalf("expected 2 tts segments, got %d (%v)", len(tts.texts), tts.texts)
	}
	if tts.texts[0] != "你好，世界。" {
		t.Fatalf("expected first segment 你好，世界。, got %q", tts.texts[0])
	}
	if tts.texts[1] != "再见" {
		t.Fatalf("expected second segment 再见, got %q", tts.texts[1])
	}
}

func TestResponseRuntimeIncludesRecentHistoryInNextCompletion(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	llm := &testLLM{
		events: []adapters.LLMEvent{
			{Text: "好的，我记住了。"},
			{Final: true},
		},
	}
	runtime := newResponseRuntime(
		"sess_1",
		manager,
		llm,
		&testTTS{},
		newControlRuntime(manager, slog.Default()),
		config.LLMSegmenterConfig{Mode: "punctuation_boundary", Punctuation: "。！？；!?;"},
		nil,
		nil,
		slog.Default(),
	)

	turn1 := ensureTestTurn(t, task)
	runtime.HandleASREvent(turn1, adapters.ASREvent{Text: "我叫张三", Final: true})
	waitTaskActive(t, task)

	turn2 := ensureTestTurn(t, task)
	runtime.HandleASREvent(turn2, adapters.ASREvent{Text: "我要订家庭套房", Final: true})
	waitTaskActive(t, task)

	llm.mu.Lock()
	defer llm.mu.Unlock()
	if len(llm.requests) != 2 {
		t.Fatalf("expected two llm requests, got %d", len(llm.requests))
	}
	second := llm.requests[1]
	if second.SystemHint == "" {
		t.Fatal("expected concise voice system hint")
	}
	if len(second.History) < 2 {
		t.Fatalf("expected prior exchange in history, got %#v", second.History)
	}
	if second.History[0].Role != "user" || second.History[0].Text != "我叫张三" {
		t.Fatalf("expected first history item to remember user name, got %#v", second.History)
	}
}

func ensureTestTurn(t *testing.T, task *session.Task) int64 {
	t.Helper()
	turnID, created, err := task.EnsureTurn()
	if err != nil || !created {
		t.Fatalf("ensure turn failed: created=%v err=%v", created, err)
	}
	return turnID
}

func waitTaskActive(t *testing.T, task *session.Task) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := task.Snapshot().State; got == session.StateActive {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected active after response pipeline, got %s", task.Snapshot().State)
}
