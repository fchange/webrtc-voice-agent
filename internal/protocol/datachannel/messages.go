package datachannel

import "errors"

const Version = "v1alpha1"

type MessageType string

const (
	TypeSessionReady       MessageType = "session.ready"
	TypeSessionEnding      MessageType = "session.ending"
	TypeTurnStarted        MessageType = "turn.started"
	TypeTurnInterruptHint  MessageType = "turn.interrupt_hint"
	TypeTurnInterrupt      MessageType = "turn.interrupt"
	TypeTurnCancelled      MessageType = "turn.cancelled"
	TypeTurnEndOfUtterance MessageType = "turn.end_of_utterance"
	TypeTurnCompleted      MessageType = "turn.completed"
	TypeBotSpeakingStart   MessageType = "bot.speaking.started"
	TypeBotSpeakingStop    MessageType = "bot.speaking.stopped"
	TypeVADStarted         MessageType = "vad.started"
	TypeVADStopped         MessageType = "vad.stopped"
	TypeASRPartial         MessageType = "asr.partial"
	TypeASRFinal           MessageType = "asr.final"
	TypeLLMPartial         MessageType = "llm.partial"
	TypeLLMFinal           MessageType = "llm.final"
	TypeTTSSegmentStarted  MessageType = "tts.segment.started"
	TypeTTSSegmentDone     MessageType = "tts.segment.completed"
	TypeError              MessageType = "error"
)

type Envelope struct {
	Version   string      `json:"version"`
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id"`
	TurnID    int64       `json:"turn_id,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Payload   any         `json:"payload,omitempty"`
}

type InterruptPayload struct {
	Reason string `json:"reason"`
}

type EndOfUtterancePayload struct {
	Source string `json:"source"`
}

type StatusPayload struct {
	Message string `json:"message,omitempty"`
}

type TranscriptPayload struct {
	Text  string `json:"text"`
	Final bool   `json:"final,omitempty"`
}

type TextSegmentPayload struct {
	Text      string `json:"text"`
	SegmentID int    `json:"segment_id,omitempty"`
	Chunks    int    `json:"chunks,omitempty"`
	Bytes     int    `json:"bytes,omitempty"`
	Final     bool   `json:"final,omitempty"`
}

func (e Envelope) Validate() error {
	if e.Version == "" {
		return errors.New("missing version")
	}
	if e.Version != Version {
		return errors.New("unsupported version")
	}
	if e.Type == "" {
		return errors.New("missing type")
	}
	if e.SessionID == "" {
		return errors.New("missing session_id")
	}
	return nil
}
