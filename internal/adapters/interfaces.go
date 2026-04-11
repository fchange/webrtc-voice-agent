package adapters

import (
	"context"
	"time"
)

type AudioChunk struct {
	Sequence  uint64
	PCM       []byte
	Timestamp time.Duration
}

type ASREvent struct {
	Text  string
	Final bool
}

type CompletionRequest struct {
	SessionID  string
	TurnID     int64
	Text       string
	SystemHint string
	History    []ConversationMessage
}

type ConversationMessage struct {
	Role string
	Text string
}

type LLMEvent struct {
	Text  string
	Final bool
}

type SynthesisRequest struct {
	SessionID string
	TurnID    int64
	Text      string
}

type TTSEvent struct {
	Chunk AudioChunk
	Final bool
}

type ProviderBundle struct {
	ASR ASRAdapter
	LLM LLMAdapter
	TTS TTSAdapter
}

type ASRAdapter interface {
	Name() string
	Transcribe(ctx context.Context, input <-chan AudioChunk) (<-chan ASREvent, error)
}

type LLMAdapter interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (<-chan LLMEvent, error)
}

type TTSAdapter interface {
	Name() string
	Synthesize(ctx context.Context, req SynthesisRequest) (<-chan TTSEvent, error)
}
