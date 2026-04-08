package bot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/audio"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type transcriptEmitter interface {
	emitASREvent(sessionID string, turnID int64, event adapters.ASREvent)
}

type asrRuntime struct {
	sessionID        string
	targetSampleRate uint32
	manager          *session.Manager
	provider         adapters.ASRAdapter
	emitter          transcriptEmitter
	logger           *slog.Logger

	mu          sync.Mutex
	input       chan adapters.AudioChunk
	cancel      context.CancelFunc
	currentTurn int64
	sequence    uint64
	sentSamples uint64
}

func newASRRuntime(
	sessionID string,
	manager *session.Manager,
	provider adapters.ASRAdapter,
	emitter transcriptEmitter,
	targetSampleRate uint32,
	logger *slog.Logger,
) *asrRuntime {
	return &asrRuntime{
		sessionID:        sessionID,
		targetSampleRate: targetSampleRate,
		manager:          manager,
		provider:         provider,
		emitter:          emitter,
		logger:           logger,
	}
}

func (r *asrRuntime) Attach(stream *audio.PCMFrameStream) {
	if stream == nil || r.provider == nil {
		return
	}

	frames, cancel := stream.Subscribe(64)
	go func() {
		defer cancel()
		for frame := range frames {
			r.consumeFrame(frame)
		}
		r.Close()
	}()
}

func (r *asrRuntime) HandleSpeechStart() {
	if r.provider == nil {
		return
	}

	task, ok := r.manager.Get(r.sessionID)
	if !ok {
		return
	}
	turnID := task.Snapshot().CurrentTurn
	if turnID == 0 {
		return
	}

	r.mu.Lock()
	if r.input != nil {
		r.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	input := make(chan adapters.AudioChunk, 64)
	events, err := r.provider.Transcribe(ctx, input)
	if err != nil {
		r.mu.Unlock()
		cancel()
		r.logger.Error("start asr provider failed", "session_id", r.sessionID, "turn_id", turnID, "provider", r.provider.Name(), "err", err)
		return
	}

	r.input = input
	r.cancel = cancel
	r.currentTurn = turnID
	r.sequence = 0
	r.sentSamples = 0
	r.mu.Unlock()

	go r.forwardEvents(turnID, input, cancel, events)
}

func (r *asrRuntime) HandleEndOfUtterance() {
	r.mu.Lock()
	input := r.input
	r.input = nil
	r.cancel = nil
	r.currentTurn = 0
	r.sequence = 0
	r.sentSamples = 0
	r.mu.Unlock()

	if input != nil {
		close(input)
	}
}

func (r *asrRuntime) Close() {
	r.mu.Lock()
	input := r.input
	cancel := r.cancel
	r.input = nil
	r.cancel = nil
	r.currentTurn = 0
	r.sequence = 0
	r.sentSamples = 0
	r.mu.Unlock()

	if input != nil {
		close(input)
	}
	if cancel != nil {
		cancel()
	}
}

func (r *asrRuntime) consumeFrame(frame audio.PCMFrame) {
	normalized, err := audio.NormalizePCMFrame(frame, r.targetSampleRate, 1)
	if err != nil {
		r.logger.Error("normalize pcm frame failed", "session_id", r.sessionID, "err", err)
		return
	}

	chunkBytes := audio.PCMToS16LE(normalized.Samples)
	chunkDuration := audio.PCMFrameDuration(normalized)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.input == nil {
		return
	}

	chunk := adapters.AudioChunk{
		Sequence:  r.sequence,
		PCM:       chunkBytes,
		Timestamp: time.Duration(r.sentSamples) * time.Second / time.Duration(r.targetSampleRate),
	}

	select {
	case r.input <- chunk:
		r.sequence++
		r.sentSamples += uint64(len(normalized.Samples))
	case <-time.After(10 * time.Millisecond):
		r.logger.Warn("dropping pcm chunk for asr", "session_id", r.sessionID, "turn_id", r.currentTurn, "provider", r.provider.Name(), "chunk_duration", chunkDuration.String())
	}
}

func (r *asrRuntime) forwardEvents(
	turnID int64,
	input chan adapters.AudioChunk,
	cancel context.CancelFunc,
	events <-chan adapters.ASREvent,
) {
	defer cancel()
	for event := range events {
		r.emitter.emitASREvent(r.sessionID, turnID, event)
	}

	r.mu.Lock()
	if r.input == input {
		r.input = nil
		r.cancel = nil
		r.currentTurn = 0
		r.sequence = 0
		r.sentSamples = 0
	}
	r.mu.Unlock()
}
