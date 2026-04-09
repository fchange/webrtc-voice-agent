package bot

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type responseRuntime struct {
	sessionID string
	manager   *session.Manager
	llm       adapters.LLMAdapter
	tts       adapters.TTSAdapter
	control   *controlRuntime
	segmenter config.LLMSegmenterConfig
	audioOut  *downlinkAudioWriter
	debugDump *ttsDebugDumper
	logger    *slog.Logger

	mu          sync.Mutex
	cancel      context.CancelFunc
	currentTurn int64
}

func newResponseRuntime(
	sessionID string,
	manager *session.Manager,
	llm adapters.LLMAdapter,
	tts adapters.TTSAdapter,
	control *controlRuntime,
	segmenter config.LLMSegmenterConfig,
	audioOut *downlinkAudioWriter,
	debugDump *ttsDebugDumper,
	logger *slog.Logger,
) *responseRuntime {
	return &responseRuntime{
		sessionID: sessionID,
		manager:   manager,
		llm:       llm,
		tts:       tts,
		control:   control,
		segmenter: segmenter,
		audioOut:  audioOut,
		debugDump: debugDump,
		logger:    logger,
	}
}

func (r *responseRuntime) HandleASREvent(turnID int64, event adapters.ASREvent) {
	if !event.Final {
		return
	}

	text := strings.TrimSpace(event.Text)
	if text == "" {
		r.completeTurn(turnID, "turn completed with empty final transcript")
		return
	}

	r.cancelCurrent("new final transcript arrived")

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancel = cancel
	r.currentTurn = turnID
	r.mu.Unlock()

	go r.run(ctx, turnID, text)
}

func (r *responseRuntime) Close() {
	r.cancelCurrent("response runtime closed")
}

func (r *responseRuntime) Interrupt() {
	r.cancelCurrent("response interrupted")
}

func (r *responseRuntime) run(ctx context.Context, turnID int64, transcript string) {
	defer r.clear(turnID)

	task, ok := r.manager.Get(r.sessionID)
	if !ok {
		return
	}
	if err := task.StartResponse(turnID); err != nil {
		r.logger.Error("start response failed", "session_id", r.sessionID, "turn_id", turnID, "err", err)
		return
	}

	llmEvents, err := r.llm.Complete(ctx, adapters.CompletionRequest{
		SessionID: r.sessionID,
		TurnID:    turnID,
		Text:      transcript,
	})
	if err != nil {
		r.logger.Error("start llm completion failed", "session_id", r.sessionID, "turn_id", turnID, "provider", r.llm.Name(), "err", err)
		r.completeTurn(turnID, "turn completed after llm start failure")
		return
	}

	r.logger.Info("response turn started", "session_id", r.sessionID, "turn_id", turnID, "transcript", transcript, "llm", r.llm.Name(), "tts", r.tts.Name())

	segmenter := newPunctuationBoundarySegmenter(r.segmenter.Punctuation)
	var fullText strings.Builder
	segmentID := 0
	speaking := false

	for event := range llmEvents {
		if event.Text != "" {
			fullText.WriteString(event.Text)
			if r.control != nil {
				r.control.emitLLMEvent(r.sessionID, turnID, adapters.LLMEvent{Text: event.Text})
			}
			for _, segment := range segmenter.Push(event.Text) {
				if !speaking {
					speaking = true
					if r.control != nil {
						r.control.emitBotSpeakingStarted(r.sessionID, turnID, "llm punctuation segment ready for tts")
					}
				}
				segmentID++
				r.synthesizeSegment(ctx, turnID, segmentID, segment, false)
			}
		}
		if event.Final {
			break
		}
	}

	if tail := segmenter.Flush(); tail != "" {
		if !speaking {
			speaking = true
			if r.control != nil {
				r.control.emitBotSpeakingStarted(r.sessionID, turnID, "llm trailing segment ready for tts")
			}
		}
		segmentID++
		r.synthesizeSegment(ctx, turnID, segmentID, tail, true)
	} else if speaking && r.control != nil {
		r.control.emitBotSpeakingStopped(r.sessionID, turnID, "tts segment synthesis drained")
	}

	if r.control != nil {
		r.control.emitLLMEvent(r.sessionID, turnID, adapters.LLMEvent{
			Text:  fullText.String(),
			Final: true,
		})
	}
	if speaking && r.audioOut != nil {
		r.audioOut.WaitIdle()
	}
	if speaking && r.control != nil {
		r.control.emitBotSpeakingStopped(r.sessionID, turnID, "response pipeline finished")
	}

	r.completeTurn(turnID, "turn completed after llm and tts pipeline")
}

func (r *responseRuntime) synthesizeSegment(ctx context.Context, turnID int64, segmentID int, text string, isFinalSegment bool) {
	r.logger.Info(
		"tts segment synthesis requested",
		"session_id", r.sessionID,
		"turn_id", turnID,
		"segment_id", segmentID,
		"text", text,
		"text_len", len(text),
		"final_segment", isFinalSegment,
		"provider", r.tts.Name(),
	)
	if r.control != nil {
		r.control.emitTTSSegmentStarted(r.sessionID, turnID, segmentID, text)
	}

	events, err := r.tts.Synthesize(ctx, adapters.SynthesisRequest{
		SessionID: r.sessionID,
		TurnID:    turnID,
		Text:      text,
	})
	if err != nil {
		r.logger.Error("start tts synthesis failed", "session_id", r.sessionID, "turn_id", turnID, "segment_id", segmentID, "provider", r.tts.Name(), "err", err)
		if r.control != nil {
			r.control.emitTTSSegmentCompleted(r.sessionID, turnID, segmentID, text, 0, 0, isFinalSegment)
		}
		return
	}

	chunks := 0
	bytes := 0
	var rawAudio []byte
	for event := range events {
		chunks++
		bytes += len(event.Chunk.PCM)
		r.logger.Info(
			"tts segment event received",
			"session_id", r.sessionID,
			"turn_id", turnID,
			"segment_id", segmentID,
			"chunk_index", chunks,
			"audio_bytes", len(event.Chunk.PCM),
			"final", event.Final,
		)
		if len(event.Chunk.PCM) > 0 {
			rawAudio = append(rawAudio, event.Chunk.PCM...)
		}
		if len(event.Chunk.PCM) > 0 && r.audioOut != nil {
			if err := r.audioOut.WritePCM16K(event.Chunk.PCM); err != nil {
				r.logger.Error("write synthesized audio to downlink failed", "session_id", r.sessionID, "turn_id", turnID, "segment_id", segmentID, "err", err)
			} else {
				r.logger.Info(
					"synthesized audio queued to downlink",
					"session_id", r.sessionID,
					"turn_id", turnID,
					"segment_id", segmentID,
					"chunk_index", chunks,
					"audio_bytes", len(event.Chunk.PCM),
				)
			}
		}
		if event.Final {
			break
		}
	}

	if r.debugDump != nil {
		if err := r.debugDump.Dump(r.sessionID, turnID, segmentID, text, rawAudio); err != nil {
			r.logger.Error("dump tts debug artifacts failed", "session_id", r.sessionID, "turn_id", turnID, "segment_id", segmentID, "err", err)
		}
	}

	if r.control != nil {
		r.control.emitTTSSegmentCompleted(r.sessionID, turnID, segmentID, text, chunks, bytes, isFinalSegment)
	}
	r.logger.Info("tts segment synthesized", "session_id", r.sessionID, "turn_id", turnID, "segment_id", segmentID, "text", text, "chunks", chunks, "bytes", bytes)
}

func (r *responseRuntime) completeTurn(turnID int64, message string) {
	task, ok := r.manager.Get(r.sessionID)
	if !ok {
		return
	}
	if err := task.CompleteTurn(turnID); err != nil {
		r.logger.Error("complete turn failed", "session_id", r.sessionID, "turn_id", turnID, "err", err)
		return
	}
	if r.control != nil {
		r.control.emitTurnCompleted(r.sessionID, turnID, message)
	}
}

func (r *responseRuntime) cancelCurrent(reason string) {
	r.mu.Lock()
	cancel := r.cancel
	turnID := r.currentTurn
	r.cancel = nil
	r.currentTurn = 0
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if r.audioOut != nil {
		r.audioOut.Flush()
	}
	if turnID != 0 && r.control != nil {
		r.control.emitBotSpeakingStopped(r.sessionID, turnID, reason)
	}
}

func (r *responseRuntime) clear(turnID int64) {
	r.mu.Lock()
	if r.currentTurn == turnID {
		r.currentTurn = 0
		r.cancel = nil
	}
	r.mu.Unlock()
}

type punctuationBoundarySegmenter struct {
	punctuation map[rune]struct{}
	buffer      strings.Builder
}

func newPunctuationBoundarySegmenter(chars string) *punctuationBoundarySegmenter {
	punctuation := make(map[rune]struct{}, len(chars))
	for _, char := range chars {
		punctuation[char] = struct{}{}
	}
	return &punctuationBoundarySegmenter{
		punctuation: punctuation,
	}
}

func (s *punctuationBoundarySegmenter) Push(text string) []string {
	var segments []string
	for _, char := range text {
		s.buffer.WriteRune(char)
		if _, ok := s.punctuation[char]; ok {
			segment := strings.TrimSpace(s.buffer.String())
			s.buffer.Reset()
			if segment != "" {
				segments = append(segments, segment)
			}
		}
	}
	return segments
}

func (s *punctuationBoundarySegmenter) Flush() string {
	segment := strings.TrimSpace(s.buffer.String())
	s.buffer.Reset()
	return segment
}
