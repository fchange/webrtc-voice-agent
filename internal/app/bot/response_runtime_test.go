package bot

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	"github.com/fchange/webrtc-voice-agent/internal/config"
	dcproto "github.com/fchange/webrtc-voice-agent/internal/protocol/datachannel"
	"github.com/fchange/webrtc-voice-agent/internal/session"
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

type blockingLLM struct {
	started chan struct{}
}

type directiveLLM struct {
	events   []adapters.LLMEvent
	reason   string
	message  string
	requests []adapters.CompletionRequest
}

func (b *blockingLLM) Name() string {
	return "blocking-llm"
}

func (b *blockingLLM) Complete(ctx context.Context, _ adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	out := make(chan adapters.LLMEvent, 1)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case out <- adapters.LLMEvent{Text: "好的。"}:
		}
		select {
		case <-ctx.Done():
		}
	}()
	if b.started != nil {
		close(b.started)
	}
	return out, nil
}

func (d *directiveLLM) Name() string {
	return "directive-llm"
}

func (d *directiveLLM) Complete(ctx context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	d.requests = append(d.requests, req)
	if directives := turnDirectivesFromContext(ctx); directives != nil {
		directives.RequestEndCall(d.reason, d.message)
	}

	out := make(chan adapters.LLMEvent, len(d.events))
	go func() {
		defer close(out)
		for _, event := range d.events {
			select {
			case <-ctx.Done():
				return
			case out <- event:
			}
		}
	}()
	return out, nil
}

type captureEnder struct {
	sessionID string
	reason    string
	message   string
	count     int
}

func (c *captureEnder) ScheduleEnd(sessionID string, reason string, message string) {
	c.sessionID = sessionID
	c.reason = reason
	c.message = message
	c.count++
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
		nil,
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
		nil,
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

func TestResponseRuntimeCancelledRunDoesNotCompleteOldTurn(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	turnID := ensureTestTurn(t, task)
	control := newControlRuntime(manager, slog.Default())
	runtime := newResponseRuntime(
		"sess_1",
		manager,
		&blockingLLM{started: make(chan struct{})},
		&testTTS{},
		control,
		nil,
		config.LLMSegmenterConfig{Mode: "punctuation_boundary", Punctuation: "。！？；!?;"},
		nil,
		nil,
		slog.Default(),
	)

	blocking := runtime.llm.(*blockingLLM)
	runtime.HandleASREvent(turnID, adapters.ASREvent{Text: "用户输入", Final: true})

	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected llm request to start")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if task.Snapshot().State == session.StateResponding {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := task.Snapshot().State; got != session.StateResponding {
		t.Fatalf("expected responding before interrupt, got %s", got)
	}

	if _, err := task.Interrupt("server_vad_barge_in"); err != nil {
		t.Fatalf("interrupt task: %v", err)
	}
	runtime.Interrupt()

	time.Sleep(100 * time.Millisecond)

	for _, envelope := range control.pending["sess_1"] {
		if envelope.Type == dcproto.TypeTurnCompleted {
			t.Fatalf("expected cancelled run not to emit turn.completed, got %+v", envelope)
		}
	}
}

func TestResponseRuntimeStartAssistantTurnCompletesGreeting(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	tts := &testTTS{}
	control := newControlRuntime(manager, slog.Default())
	runtime := newResponseRuntime(
		"sess_1",
		manager,
		&testLLM{},
		tts,
		control,
		nil,
		config.LLMSegmenterConfig{Mode: "punctuation_boundary", Punctuation: "。！？；!?;"},
		nil,
		nil,
		slog.Default(),
	)

	if err := runtime.StartAssistantTurn("您好，欢迎致电。"); err != nil {
		t.Fatalf("start assistant turn: %v", err)
	}

	waitTaskActive(t, task)

	tts.mu.Lock()
	defer tts.mu.Unlock()
	if len(tts.texts) != 1 || tts.texts[0] != "您好，欢迎致电。" {
		t.Fatalf("expected greeting to be synthesized once, got %v", tts.texts)
	}

	foundStarted := false
	foundCompleted := false
	for _, envelope := range control.pending["sess_1"] {
		if envelope.Type == dcproto.TypeTurnStarted {
			foundStarted = true
		}
		if envelope.Type == dcproto.TypeTurnCompleted {
			foundCompleted = true
		}
	}
	if !foundStarted || !foundCompleted {
		t.Fatalf("expected opening turn events, got %+v", control.pending["sess_1"])
	}
}

func TestResponseRuntimeSchedulesSessionEndAfterToolDirective(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	turnID := ensureTestTurn(t, task)
	ender := &captureEnder{}
	runtime := newResponseRuntime(
		"sess_1",
		manager,
		&directiveLLM{
			reason:  "reservation_confirmed",
			message: "本次预订已完成，通话即将结束。",
			events: []adapters.LLMEvent{
				{Text: "花园双床房已为您预订成功，确认号 res_000001。"},
				{Final: true},
			},
		},
		&testTTS{},
		newControlRuntime(manager, slog.Default()),
		ender,
		config.LLMSegmenterConfig{Mode: "punctuation_boundary", Punctuation: "。！？；!?;"},
		nil,
		nil,
		slog.Default(),
	)

	runtime.HandleASREvent(turnID, adapters.ASREvent{Text: "帮我预订", Final: true})
	waitTaskActive(t, task)

	if ender.count != 1 {
		t.Fatalf("expected one scheduled end, got %d", ender.count)
	}
	if ender.sessionID != "sess_1" || ender.reason != "reservation_confirmed" {
		t.Fatalf("unexpected end request: %+v", ender)
	}
}

func TestResponseRuntimeIgnoresEndCallDirectiveWhenReplyIsNotTerminal(t *testing.T) {
	manager := session.NewManager(time.Minute)
	task := manager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	turnID := ensureTestTurn(t, task)
	ender := &captureEnder{}
	runtime := newResponseRuntime(
		"sess_1",
		manager,
		&directiveLLM{
			reason:  "bot_farewell",
			message: "Bot 已完成结束语，通话即将结束。",
			events: []adapters.LLMEvent{
				{Text: "请告诉我您的姓名和手机号。"},
				{Final: true},
			},
		},
		&testTTS{},
		newControlRuntime(manager, slog.Default()),
		ender,
		config.LLMSegmenterConfig{Mode: "punctuation_boundary", Punctuation: "。！？；!?;"},
		nil,
		nil,
		slog.Default(),
	)

	runtime.HandleASREvent(turnID, adapters.ASREvent{Text: "帮我预订", Final: true})
	waitTaskActive(t, task)

	if ender.count != 0 {
		t.Fatalf("expected no scheduled end for non-terminal reply, got %+v", ender)
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
