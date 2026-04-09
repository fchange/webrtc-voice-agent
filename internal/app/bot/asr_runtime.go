package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/audio"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type transcriptEmitter interface {
	emitASREvent(sessionID string, turnID int64, event adapters.ASREvent)
}

type runtimeErrorEmitter interface {
	emitError(sessionID string, turnID int64, message string)
}

type asrEventHandler interface {
	HandleASREvent(turnID int64, event adapters.ASREvent)
}

type asrRuntime struct {
	sessionID        string
	targetSampleRate uint32
	manager          *session.Manager
	provider         adapters.ASRAdapter
	emitter          transcriptEmitter
	handler          asrEventHandler
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

const (
	stablePartialRepeatThreshold = 4
	stablePartialTimeout         = 1500 * time.Millisecond
)

func newASRRuntime(
	sessionID string,
	manager *session.Manager,
	provider adapters.ASRAdapter,
	emitter transcriptEmitter,
	handler asrEventHandler,
	targetSampleRate uint32,
	logger *slog.Logger,
) *asrRuntime {
	return &asrRuntime{
		sessionID:        sessionID,
		targetSampleRate: targetSampleRate,
		manager:          manager,
		provider:         provider,
		emitter:          emitter,
		handler:          handler,
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
		if emitter, ok := r.emitter.(runtimeErrorEmitter); ok {
			emitter.emitError(r.sessionID, turnID, fmt.Sprintf("ASR provider %s failed to start: %v", r.provider.Name(), err))
		}
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
	lastPartialText := ""
	lastPartialAt := time.Time{}
	repeatCount := 0
	stablePromoted := false
	lastNonEmptyText := ""
	sawFinal := false

	for event := range events {
		if stablePromoted {
			continue
		}

		if text := strings.TrimSpace(event.Text); text != "" {
			lastNonEmptyText = text
		}

		if !event.Final && event.Text != "" {
			now := time.Now().UTC()
			if event.Text == lastPartialText {
				repeatCount++
				if shouldPromoteStablePartial(event.Text, repeatCount, lastPartialAt, now) {
					finalEvent := adapters.ASREvent{
						Text:  event.Text,
						Final: true,
					}
					r.logger.Info(
						"promoting stable asr partial to final",
						"session_id", r.sessionID,
						"turn_id", turnID,
						"provider", r.provider.Name(),
						"repeat_count", repeatCount,
						"text_len", len(event.Text),
						"text", event.Text,
					)
					r.emitter.emitASREvent(r.sessionID, turnID, finalEvent)
					if r.handler != nil {
						r.handler.HandleASREvent(turnID, finalEvent)
					}
					r.HandleEndOfUtterance()
					stablePromoted = true
					continue
				}
			} else {
				lastPartialText = event.Text
				lastPartialAt = now
				repeatCount = 1
			}
		}

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
		if r.handler != nil {
			r.handler.HandleASREvent(turnID, event)
		}
		if event.Final {
			sawFinal = true
			r.logger.Info(
				"provider asr final received; closing turn input",
				"session_id", r.sessionID,
				"turn_id", turnID,
				"provider", r.provider.Name(),
			)
			r.HandleEndOfUtterance()
			stablePromoted = true
		}
	}

	if !stablePromoted && !sawFinal && lastNonEmptyText != "" {
		finalEvent := adapters.ASREvent{
			Text:  lastNonEmptyText,
			Final: true,
		}
		r.logger.Info(
			"promoting trailing asr partial to final after provider stream ended",
			"session_id", r.sessionID,
			"turn_id", turnID,
			"provider", r.provider.Name(),
			"text_len", len(lastNonEmptyText),
			"text", lastNonEmptyText,
		)
		r.emitter.emitASREvent(r.sessionID, turnID, finalEvent)
		if r.handler != nil {
			r.handler.HandleASREvent(turnID, finalEvent)
		}
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

func shouldPromoteStablePartial(text string, repeatCount int, lastPartialAt time.Time, now time.Time) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	if repeatCount < stablePartialRepeatThreshold {
		return false
	}
	if now.Sub(lastPartialAt) < stablePartialTimeout {
		return false
	}

	return strings.ContainsAny(text, "。！？!?")
}
