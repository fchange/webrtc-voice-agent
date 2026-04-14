package bot

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	dcproto "github.com/fchange/webrtc-voice-agent/internal/protocol/datachannel"
	"github.com/fchange/webrtc-voice-agent/internal/session"
	"github.com/pion/webrtc/v4"
)

type controlRuntime struct {
	manager     *session.Manager
	logger      *slog.Logger
	mu          sync.Mutex
	channels    map[string]*webrtc.DataChannel
	pending     map[string][]dcproto.Envelope
	onInterrupt func(sessionID string)
	onReady     func(sessionID string)
}

func newControlRuntime(manager *session.Manager, logger *slog.Logger) *controlRuntime {
	return &controlRuntime{
		manager:  manager,
		logger:   logger,
		channels: make(map[string]*webrtc.DataChannel),
		pending:  make(map[string][]dcproto.Envelope),
	}
}

const maxPendingControlEvents = 64

func (r *controlRuntime) setInterruptHandler(handler func(sessionID string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onInterrupt = handler
}

func (r *controlRuntime) setReadyHandler(handler func(sessionID string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onReady = handler
}

func (r *controlRuntime) bind(sessionID string, channel *webrtc.DataChannel) {
	channel.OnOpen(func() {
		task, ok := r.manager.Get(sessionID)
		if !ok {
			r.sendError(channel, sessionID, "session not found")
			return
		}

		if err := task.MarkActive(); err != nil {
			r.logger.Info("session already active on datachannel open", "session_id", sessionID, "err", err)
		}
		r.mu.Lock()
		r.channels[sessionID] = channel
		pending := append([]dcproto.Envelope(nil), r.pending[sessionID]...)
		delete(r.pending, sessionID)
		r.mu.Unlock()

		r.send(channel, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeSessionReady,
			SessionID: sessionID,
			Payload: dcproto.StatusPayload{
				Message: "session ready",
			},
		})
		for _, envelope := range pending {
			r.send(channel, envelope)
		}

		r.mu.Lock()
		handler := r.onReady
		r.mu.Unlock()
		if handler != nil {
			handler(sessionID)
		}
	})

	channel.OnClose(func() {
		r.mu.Lock()
		if current, ok := r.channels[sessionID]; ok && current == channel {
			delete(r.channels, sessionID)
		}
		r.mu.Unlock()
	})

	channel.OnMessage(func(msg webrtc.DataChannelMessage) {
		var envelope dcproto.Envelope
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			r.sendError(channel, sessionID, "invalid datachannel payload")
			return
		}
		if envelope.SessionID == "" {
			envelope.SessionID = sessionID
		}
		if err := envelope.Validate(); err != nil {
			r.sendError(channel, sessionID, err.Error())
			return
		}

		if err := r.handleEnvelope(channel, envelope); err != nil {
			r.logger.Error("control runtime error", "session_id", sessionID, "type", envelope.Type, "err", err)
			r.sendError(channel, sessionID, err.Error())
		}
	})
}

func (r *controlRuntime) handleEnvelope(channel *webrtc.DataChannel, envelope dcproto.Envelope) error {
	task, ok := r.manager.Get(envelope.SessionID)
	if !ok {
		return fmt.Errorf("session not found")
	}

	switch envelope.Type {
	case dcproto.TypeTurnInterruptHint:
		return r.handleInterruptHint(channel, task, envelope)
	default:
		r.logger.Info("ignoring unsupported control message", "session_id", envelope.SessionID, "type", envelope.Type)
		return nil
	}
}

func (r *controlRuntime) emitASREvent(sessionID string, turnID int64, event adapters.ASREvent) {
	messageType := dcproto.TypeASRPartial
	if event.Final {
		messageType = dcproto.TypeASRFinal
	}

	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      messageType,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.TranscriptPayload{
			Text:  event.Text,
			Final: event.Final,
		},
	})
}

func (r *controlRuntime) emitLLMEvent(sessionID string, turnID int64, event adapters.LLMEvent) {
	messageType := dcproto.TypeLLMPartial
	if event.Final {
		messageType = dcproto.TypeLLMFinal
	}

	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      messageType,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.TranscriptPayload{
			Text:  event.Text,
			Final: event.Final,
		},
	})
}

func (r *controlRuntime) emitTTSSegmentStarted(sessionID string, turnID int64, segmentID int, text string) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeTTSSegmentStarted,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.TextSegmentPayload{
			Text:      text,
			SegmentID: segmentID,
		},
	})
}

func (r *controlRuntime) emitTTSSegmentCompleted(sessionID string, turnID int64, segmentID int, text string, chunks int, bytes int, final bool) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeTTSSegmentDone,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.TextSegmentPayload{
			Text:      text,
			SegmentID: segmentID,
			Chunks:    chunks,
			Bytes:     bytes,
			Final:     final,
		},
	})
}

func (r *controlRuntime) emitBotSpeakingStarted(sessionID string, turnID int64, message string) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeBotSpeakingStart,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.StatusPayload{
			Message: message,
		},
	})
}

func (r *controlRuntime) emitBotSpeakingStopped(sessionID string, turnID int64, message string) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeBotSpeakingStop,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.StatusPayload{
			Message: message,
		},
	})
}

func (r *controlRuntime) emitTurnCompleted(sessionID string, turnID int64, message string) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeTurnCompleted,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.StatusPayload{
			Message: message,
		},
	})
}

func (r *controlRuntime) emitSessionEnding(sessionID string, message string) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeSessionEnding,
		SessionID: sessionID,
		Payload: dcproto.StatusPayload{
			Message: message,
		},
	})
}

func (r *controlRuntime) emitError(sessionID string, turnID int64, message string) {
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeError,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.StatusPayload{
			Message: message,
		},
	})
}

func (r *controlRuntime) handleVADStart(sessionID string) {
	task, ok := r.manager.Get(sessionID)
	if !ok {
		return
	}

	snapshot := task.Snapshot()
	if snapshot.State == session.StateResponding {
		result, err := task.Interrupt("server_vad_barge_in")
		if err != nil {
			r.logger.Error("server vad interrupt failed", "session_id", sessionID, "turn_id", snapshot.CurrentTurn, "err", err)
			return
		}

		r.logger.Info("emitting bot.speaking.stopped from server vad barge-in", "session_id", sessionID, "turn_id", result.InterruptedTurnID)
		r.emit(sessionID, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeBotSpeakingStop,
			SessionID: sessionID,
			TurnID:    result.InterruptedTurnID,
			Payload: dcproto.StatusPayload{
				Message: "bot speaking stopped by server vad barge-in",
			},
		})
		r.logger.Info("emitting turn.interrupt from server vad barge-in", "session_id", sessionID, "turn_id", result.InterruptedTurnID, "next_turn_id", result.NextTurnID)
		r.emit(sessionID, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeTurnInterrupt,
			SessionID: sessionID,
			TurnID:    result.InterruptedTurnID,
			Payload:   result,
		})

		r.mu.Lock()
		handler := r.onInterrupt
		r.mu.Unlock()
		if handler != nil {
			handler(sessionID)
		}
	}

	turnID, created, err := task.EnsureTurn()
	if err != nil {
		r.logger.Error("server vad start failed", "session_id", sessionID, "err", err)
		return
	}
	if created {
		r.logger.Info("emitting turn.started from server endpointing", "session_id", sessionID, "turn_id", turnID)
		r.emit(sessionID, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeTurnStarted,
			SessionID: sessionID,
			TurnID:    turnID,
			Payload: dcproto.StatusPayload{
				Message: "turn started by server endpointing",
			},
		})
	}

	r.logger.Info("emitting vad.started", "session_id", sessionID, "turn_id", turnID)
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeVADStarted,
		SessionID: sessionID,
		TurnID:    turnID,
		Payload: dcproto.StatusPayload{
			Message: "server endpointing detected speech activity",
		},
	})
}

func (r *controlRuntime) handleVADEnd(sessionID string) {
	task, ok := r.manager.Get(sessionID)
	if !ok {
		return
	}

	snapshot := task.Snapshot()
	if snapshot.CurrentTurn == 0 {
		return
	}
	if snapshot.State != session.StateProcessing && snapshot.State != session.StateResponding {
		return
	}

	r.logger.Info("emitting vad.stopped", "session_id", sessionID, "turn_id", snapshot.CurrentTurn, "state", snapshot.State)
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeVADStopped,
		SessionID: sessionID,
		TurnID:    snapshot.CurrentTurn,
		Payload: dcproto.StatusPayload{
			Message: "server endpointing detected silence",
		},
	})
}

func (r *controlRuntime) handleEndOfUtterance(sessionID string) {
	task, ok := r.manager.Get(sessionID)
	if !ok {
		return
	}

	snapshot := task.Snapshot()
	if snapshot.CurrentTurn == 0 {
		return
	}
	if snapshot.State != session.StateProcessing && snapshot.State != session.StateResponding {
		return
	}

	r.logger.Info("emitting turn.end_of_utterance", "session_id", sessionID, "turn_id", snapshot.CurrentTurn, "state", snapshot.State)
	r.emit(sessionID, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeTurnEndOfUtterance,
		SessionID: sessionID,
		TurnID:    snapshot.CurrentTurn,
		Payload: dcproto.EndOfUtterancePayload{
			Source: "server_packet_endpointing",
		},
	})
}

func (r *controlRuntime) handleInterruptHint(channel *webrtc.DataChannel, task *session.Task, envelope dcproto.Envelope) error {
	var payload dcproto.InterruptPayload
	if envelope.Payload != nil {
		raw, err := json.Marshal(envelope.Payload)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
	}
	if payload.Reason == "" {
		payload.Reason = "user_barge_in"
	}

	turnID, created, err := task.EnsureTurn()
	if err != nil {
		return err
	}
	if created {
		r.send(channel, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeTurnStarted,
			SessionID: envelope.SessionID,
			TurnID:    turnID,
			Payload: dcproto.StatusPayload{
				Message: "turn started from interrupt hint",
			},
		})
		if err := task.StartResponse(turnID); err != nil {
			return err
		}
		r.send(channel, dcproto.Envelope{
			Version:   dcproto.Version,
			Type:      dcproto.TypeBotSpeakingStart,
			SessionID: envelope.SessionID,
			TurnID:    turnID,
			Payload: dcproto.StatusPayload{
				Message: "bot speaking placeholder",
			},
		})
	}

	result, err := task.Interrupt(payload.Reason)
	if err != nil {
		return err
	}

	r.send(channel, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeBotSpeakingStop,
		SessionID: envelope.SessionID,
		TurnID:    result.InterruptedTurnID,
		Payload: dcproto.StatusPayload{
			Message: "bot speaking stopped by interrupt",
		},
	})
	r.send(channel, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeTurnInterrupt,
		SessionID: envelope.SessionID,
		TurnID:    result.InterruptedTurnID,
		Payload:   result,
	})

	r.mu.Lock()
	handler := r.onInterrupt
	r.mu.Unlock()
	if handler != nil {
		handler(envelope.SessionID)
	}

	return nil
}

func (r *controlRuntime) send(channel *webrtc.DataChannel, envelope dcproto.Envelope) {
	data, err := json.Marshal(envelope)
	if err != nil {
		r.logger.Error("marshal control envelope failed", "session_id", envelope.SessionID, "type", envelope.Type, "err", err)
		return
	}
	if err := channel.SendText(string(data)); err != nil {
		r.logger.Error("send control envelope failed", "session_id", envelope.SessionID, "type", envelope.Type, "err", err)
	}
}

func (r *controlRuntime) emit(sessionID string, envelope dcproto.Envelope) {
	r.mu.Lock()
	channel := r.channels[sessionID]
	if channel == nil {
		r.pending[sessionID] = appendPendingEnvelope(r.pending[sessionID], envelope)
	}
	r.mu.Unlock()

	if channel == nil {
		return
	}
	r.send(channel, envelope)
}

func appendPendingEnvelope(events []dcproto.Envelope, envelope dcproto.Envelope) []dcproto.Envelope {
	events = append(events, envelope)
	if len(events) <= maxPendingControlEvents {
		return events
	}
	return append([]dcproto.Envelope(nil), events[len(events)-maxPendingControlEvents:]...)
}

func (r *controlRuntime) sendError(channel *webrtc.DataChannel, sessionID string, message string) {
	r.send(channel, dcproto.Envelope{
		Version:   dcproto.Version,
		Type:      dcproto.TypeError,
		SessionID: sessionID,
		Payload: dcproto.StatusPayload{
			Message: message,
		},
	})
}
