package mock

import (
	"context"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
)

type ProviderBundle struct {
	ASR adapters.ASRAdapter
	LLM adapters.LLMAdapter
	TTS adapters.TTSAdapter
}

type ASR struct{}

func NewASR() ASR {
	return ASR{}
}

func (ASR) Name() string {
	return "mock-asr"
}

func (ASR) Transcribe(ctx context.Context, input <-chan adapters.AudioChunk) (<-chan adapters.ASREvent, error) {
	out := make(chan adapters.ASREvent, 1)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
			out <- adapters.ASREvent{Text: "mock transcript", Final: true}
		}
	}()
	return out, nil
}

type LLM struct{}

func NewLLM() LLM {
	return LLM{}
}

func (LLM) Name() string {
	return "mock-llm"
}

func (LLM) Complete(ctx context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	out := make(chan adapters.LLMEvent, 1)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
			out <- adapters.LLMEvent{Text: "mock reply", Final: true}
		}
	}()
	return out, nil
}

type TTS struct{}

func NewTTS() TTS {
	return TTS{}
}

func (TTS) Name() string {
	return "mock-tts"
}

func (TTS) Synthesize(ctx context.Context, req adapters.SynthesisRequest) (<-chan adapters.TTSEvent, error) {
	out := make(chan adapters.TTSEvent, 1)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
			out <- adapters.TTSEvent{
				Chunk: adapters.AudioChunk{Sequence: 1, PCM: []byte("mock"), Timestamp: 0},
				Final: true,
			}
		}
	}()
	return out, nil
}
