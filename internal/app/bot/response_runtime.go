package bot

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	dcproto "github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/datachannel"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type responseRuntime struct {
	sessionID string
	manager   *session.Manager
	llm       adapters.LLMAdapter
	tts       adapters.TTSAdapter
	control   *controlRuntime
	ender     sessionEnder
	segmenter config.LLMSegmenterConfig
	audioOut  *downlinkAudioWriter
	debugDump *ttsDebugDumper
	logger    *slog.Logger

	mu          sync.Mutex
	cancel      context.CancelFunc
	currentTurn int64
	history     []adapters.ConversationMessage
}

const maxResponseHistoryMessages = 8

const voiceResponseSystemHint = "电话语音回复要求：必须简短自然，通常一句话，不超过35个汉字；不要寒暄过多，不要重复列清单；记住上文用户已提供的姓名、手机号、房型，不要重复索要。如果你判断当前这句说完后就该结束通话，必须先调用 end_call，再输出最后一句结束语；如果还需要继续追问、确认或解释，就不要调用 end_call。"
const openingGreetingText = "您好，这里是酒店预订服务，我可以帮您查房型和办理预订，请问您想订什么房型？"

type sessionEnder interface {
	ScheduleEnd(sessionID string, reason string, message string)
}

func newResponseRuntime(
	sessionID string,
	manager *session.Manager,
	llm adapters.LLMAdapter,
	tts adapters.TTSAdapter,
	control *controlRuntime,
	ender sessionEnder,
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
		ender:     ender,
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

func (r *responseRuntime) StartAssistantTurn(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	task, ok := r.manager.Get(r.sessionID)
	if !ok {
		return nil
	}

	turnID, err := task.StartTurn()
	if err != nil {
		return err
	}
	if r.control != nil {
		r.control.emit(r.sessionID, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeTurnStarted,
			SessionID: r.sessionID,
			TurnID:    turnID,
			Payload: dcproto.StatusPayload{
				Message: "assistant opening turn started",
			},
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancel = cancel
	r.currentTurn = turnID
	r.mu.Unlock()

	go r.runAssistantTurn(ctx, turnID, text)
	return nil
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

	directives := newTurnDirectives()
	llmEvents, err := r.llm.Complete(withTurnDirectives(ctx, directives), adapters.CompletionRequest{
		SessionID:  r.sessionID,
		TurnID:     turnID,
		Text:       transcript,
		SystemHint: voiceResponseSystemHint,
		History:    r.historySnapshot(),
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

	if ctx.Err() != nil {
		r.logger.Info("response turn cancelled before tts finalization", "session_id", r.sessionID, "turn_id", turnID, "err", ctx.Err())
		return
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

	if ctx.Err() != nil {
		r.logger.Info("response turn cancelled before completion", "session_id", r.sessionID, "turn_id", turnID, "err", ctx.Err())
		return
	}

	if r.control != nil {
		r.control.emitLLMEvent(r.sessionID, turnID, adapters.LLMEvent{
			Text:  fullText.String(),
			Final: true,
		})
	}
	r.rememberExchange(transcript, fullText.String())
	if speaking && r.audioOut != nil {
		r.audioOut.WaitIdle()
	}
	if ctx.Err() != nil {
		r.logger.Info("response turn cancelled after audio drain", "session_id", r.sessionID, "turn_id", turnID, "err", ctx.Err())
		return
	}
	if speaking && r.control != nil {
		r.control.emitBotSpeakingStopped(r.sessionID, turnID, "response pipeline finished")
	}

	r.completeTurn(turnID, "turn completed after llm and tts pipeline")
	if endCall, ok := directives.EndCall(); ok && r.ender != nil {
		if shouldScheduleEndCall(fullText.String(), endCall.Reason) {
			r.logger.Info(
				"scheduling session end after current reply",
				"session_id", r.sessionID,
				"turn_id", turnID,
				"reason", endCall.Reason,
				"message", endCall.Message,
			)
			r.ender.ScheduleEnd(r.sessionID, endCall.Reason, endCall.Message)
		} else {
			r.logger.Info(
				"skip end_call because reply does not look terminal",
				"session_id", r.sessionID,
				"turn_id", turnID,
				"reason", endCall.Reason,
				"text", fullText.String(),
			)
		}
	}
}

func (r *responseRuntime) runAssistantTurn(ctx context.Context, turnID int64, text string) {
	defer r.clear(turnID)

	task, ok := r.manager.Get(r.sessionID)
	if !ok {
		return
	}
	if err := task.StartResponse(turnID); err != nil {
		r.logger.Error("start assistant turn failed", "session_id", r.sessionID, "turn_id", turnID, "err", err)
		return
	}

	if r.control != nil {
		r.control.emitBotSpeakingStarted(r.sessionID, turnID, "assistant opening greeting ready for tts")
	}
	r.synthesizeSegment(ctx, turnID, 1, text, true)
	if ctx.Err() != nil {
		r.logger.Info("assistant turn cancelled before completion", "session_id", r.sessionID, "turn_id", turnID, "err", ctx.Err())
		return
	}

	r.rememberExchange("", text)
	if r.audioOut != nil {
		r.audioOut.WaitIdle()
	}
	if ctx.Err() != nil {
		r.logger.Info("assistant turn cancelled after audio drain", "session_id", r.sessionID, "turn_id", turnID, "err", ctx.Err())
		return
	}
	if r.control != nil {
		r.control.emitBotSpeakingStopped(r.sessionID, turnID, "assistant opening greeting finished")
	}
	r.completeTurn(turnID, "turn completed after assistant opening greeting")
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

func (r *responseRuntime) historySnapshot() []adapters.ConversationMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]adapters.ConversationMessage(nil), r.history...)
}

func (r *responseRuntime) rememberExchange(userText string, assistantText string) {
	userText = strings.TrimSpace(userText)
	assistantText = strings.TrimSpace(assistantText)
	if userText == "" && assistantText == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if userText != "" {
		r.history = append(r.history, adapters.ConversationMessage{Role: "user", Text: userText})
	}
	if assistantText != "" {
		r.history = append(r.history, adapters.ConversationMessage{Role: "assistant", Text: assistantText})
	}
	if len(r.history) > maxResponseHistoryMessages {
		r.history = append([]adapters.ConversationMessage(nil), r.history[len(r.history)-maxResponseHistoryMessages:]...)
	}
}

func shouldScheduleEndCall(text string, reason string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	switch strings.TrimSpace(reason) {
	case "reservation_confirmed":
		return hotelTextLooksConfirmed(text) || strings.Contains(text, "确认号")
	case "user_declined", "bot_farewell":
		return hotelTextLooksFarewell(text)
	default:
		return hotelTextLooksConfirmed(text) || strings.Contains(text, "确认号") || hotelTextLooksFarewell(text)
	}
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
