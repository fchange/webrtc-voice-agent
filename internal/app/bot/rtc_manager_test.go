package bot

import (
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/mock"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	dcproto "github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/datachannel"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/signaling"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

type captureWriter struct {
	mu        sync.Mutex
	envelopes []signaling.Envelope
}

func (w *captureWriter) WriteEnvelope(envelope signaling.Envelope) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.envelopes = append(w.envelopes, envelope)
	return nil
}

func (w *captureWriter) find(messageType signaling.MessageType) (signaling.Envelope, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, envelope := range w.envelopes {
		if envelope.Type == messageType {
			return envelope, true
		}
	}
	return signaling.Envelope{}, false
}

func TestRTCManagerHandleOfferProducesAnswer(t *testing.T) {
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("new client pc: %v", err)
	}
	defer client.Close()

	if _, err := client.CreateDataChannel("control", nil); err != nil {
		t.Fatalf("create data channel: %v", err)
	}
	if _, err := client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatalf("add transceiver: %v", err)
	}

	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	if err := client.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description: %v", err)
	}

	sessionManager := session.NewManager(time.Minute)
	control := newControlRuntime(sessionManager, slog.Default())
	manager := newRTCManager("", slog.Default(), sessionManager, control, mock.NewASR(), mock.NewLLM(), mock.NewTTS(), 16000, 900*time.Millisecond, config.LLMSegmenterConfig{}, config.XFYUNTTSConfig{})
	writer := &captureWriter{}

	if err := manager.HandleOffer("sess_1", signaling.SDPPayload{
		SDP:  offer.SDP,
		Type: "offer",
	}, writer); err != nil {
		t.Fatalf("handle offer: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		envelope, ok := writer.find(signaling.TypeSessionAnswer)
		if ok {
			var payload signaling.SDPPayload
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatalf("unmarshal answer: %v", err)
			}
			if payload.Type != "answer" {
				t.Fatalf("expected answer type, got %q", payload.Type)
			}
			if payload.SDP == "" {
				t.Fatal("expected non-empty SDP in answer")
			}
			manager.Close("sess_1")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected answer envelope")
}

func TestRTCManagerScheduleEndEmitsSessionEndingAndSignalClose(t *testing.T) {
	sessionManager := session.NewManager(time.Minute)
	task := sessionManager.Create("sess_1")
	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	control := newControlRuntime(sessionManager, slog.Default())
	manager := newRTCManager("", slog.Default(), sessionManager, control, mock.NewASR(), mock.NewLLM(), mock.NewTTS(), 16000, 900*time.Millisecond, config.LLMSegmenterConfig{}, config.XFYUNTTSConfig{})
	writer := &captureWriter{}
	manager.session["sess_1"] = &rtcSession{signal: writer}

	manager.ScheduleEnd("sess_1", "reservation_confirmed", "本次预订已完成，通话即将结束。")

	pending := control.pending["sess_1"]
	if len(pending) != 1 || pending[0].Type != dcproto.TypeSessionEnding {
		t.Fatalf("expected session.ending pending event, got %+v", pending)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := writer.find(signaling.TypeSessionClose); ok {
			if _, exists := sessionManager.Get("sess_1"); exists {
				t.Fatal("expected manager session to be ended")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("expected session.close envelope")
}
