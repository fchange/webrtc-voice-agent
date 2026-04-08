package events

import "time"

type Priority int

const (
	PriorityNormal Priority = iota
	PriorityHigh
	PriorityCritical
)

type Kind string

const (
	KindAudio  Kind = "audio"
	KindText   Kind = "text"
	KindStatus Kind = "status"
	KindSystem Kind = "system"
)

const (
	TypeVADStarted         = "vad.started"
	TypeVADStopped         = "vad.stopped"
	TypeTurnInterruptHint  = "turn.interrupt_hint"
	TypeTurnInterrupt      = "turn.interrupt"
	TypeTurnEndOfUtterance = "turn.end_of_utterance"
	TypeBotSpeakingStarted = "bot.speaking.started"
	TypeBotSpeakingStopped = "bot.speaking.stopped"
)

type Event struct {
	SessionID string         `json:"session_id"`
	TurnID    int64          `json:"turn_id,omitempty"`
	Kind      Kind           `json:"kind"`
	Type      string         `json:"type"`
	Priority  Priority       `json:"priority"`
	Time      time.Time      `json:"time"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func NewSystemEvent(sessionID string, turnID int64, eventType string, priority Priority) Event {
	return Event{
		SessionID: sessionID,
		TurnID:    turnID,
		Kind:      KindSystem,
		Type:      eventType,
		Priority:  priority,
		Time:      time.Now().UTC(),
	}
}

func NewStatusEvent(sessionID string, turnID int64, eventType string, payload map[string]any) Event {
	return Event{
		SessionID: sessionID,
		TurnID:    turnID,
		Kind:      KindStatus,
		Type:      eventType,
		Priority:  PriorityNormal,
		Time:      time.Now().UTC(),
		Payload:   payload,
	}
}
