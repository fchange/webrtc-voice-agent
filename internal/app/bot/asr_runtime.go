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
	chunkCount  uint64
	byteCount   uint64
	startedAt   time.Time
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
	r.chunkCount = 0
	r.byteCount = 0
	r.startedAt = time.Now().UTC()
	r.mu.Unlock()

	r.logger.Info(
		"asr turn started",
		"session_id", r.sessionID,
		"turn_id", turnID,
		"provider", r.provider.Name(),
		"sample_rate", r.targetSampleRate,
	)

	go r.forwardEvents(turnID, input, cancel, events)
}

func (r *asrRuntime) HandleEndOfUtterance() {
	r.mu.Lock()
	input := r.input
	turnID := r.currentTurn
	chunkCount := r.chunkCount
	byteCount := r.byteCount
	sentSamples := r.sentSamples
	startedAt := r.startedAt
	r.input = nil
	r.cancel = nil
	r.currentTurn = 0
	r.sequence = 0
	r.sentSamples = 0
	r.chunkCount = 0
	r.byteCount = 0
	r.startedAt = time.Time{}
	r.mu.Unlock()

	if input != nil {
		r.logger.Info(
			"asr turn input closed",
			"session_id", r.sessionID,
			"turn_id", turnID,
			"provider", r.provider.Name(),
			"chunks", chunkCount,
			"bytes", byteCount,
			"samples", sentSamples,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
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
	r.chunkCount = 0
	r.byteCount = 0
	r.startedAt = time.Time{}
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
		if r.chunkCount == 0 {
			r.logger.Info(
				"asr first pcm chunk queued",
				"session_id", r.sessionID,
				"turn_id", r.currentTurn,
				"provider", r.provider.Name(),
				"bytes", len(chunk.PCM),
				"timestamp_ms", chunk.Timestamp.Milliseconds(),
			)
		}
		r.sequence++
		r.sentSamples += uint64(len(normalized.Samples))
		r.chunkCount++
		r.byteCount += uint64(len(chunk.PCM))
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
		r.logger.Info(
			"asr event received",
			"session_id", r.sessionID,
			"turn_id", turnID,
			"provider", r.provider.Name(),
			"final", event.Final,
			"text_len", len(event.Text),
			"text", event.Text,
		)
		r.emitter.emitASREvent(r.sessionID, turnID, event)
	}

	r.mu.Lock()
	if r.input == input {
		r.input = nil
		r.cancel = nil
		r.currentTurn = 0
		r.sequence = 0
		r.sentSamples = 0
		r.chunkCount = 0
		r.byteCount = 0
		r.startedAt = time.Time{}
	}
	r.mu.Unlock()
}
