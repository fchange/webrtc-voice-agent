package signal

import (
	"testing"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/signaling"
)

func TestHubRelayToAttachedPeer(t *testing.T) {
	h := newHub()
	h.createSession("sess_1")

	client, _, err := h.attach("sess_1", roleClient, nil)
	if err != nil {
		t.Fatalf("attach client: %v", err)
	}
	bot, _, err := h.attach("sess_1", roleBot, nil)
	if err != nil {
		t.Fatalf("attach bot: %v", err)
	}

	envelope := signaling.Envelope{
		Version:   signaling.Version,
		Type:      signaling.TypeSessionOffer,
		SessionID: "sess_1",
	}
	if err := h.relay("sess_1", client.role, envelope); err != nil {
		t.Fatalf("relay: %v", err)
	}

	select {
	case got := <-bot.send:
		if got.Type != signaling.TypeSessionOffer {
			t.Fatalf("expected offer, got %s", got.Type)
		}
	default:
		t.Fatal("expected relayed message for bot")
	}
}

func TestHubQueuesUntilPeerAttached(t *testing.T) {
	h := newHub()
	h.createSession("sess_1")

	client, _, err := h.attach("sess_1", roleClient, nil)
	if err != nil {
		t.Fatalf("attach client: %v", err)
	}

	envelope := signaling.Envelope{
		Version:   signaling.Version,
		Type:      signaling.TypeSessionOffer,
		SessionID: "sess_1",
	}
	if err := h.relay("sess_1", client.role, envelope); err != nil {
		t.Fatalf("relay: %v", err)
	}

	_, queued, err := h.attach("sess_1", roleBot, nil)
	if err != nil {
		t.Fatalf("attach bot: %v", err)
	}

	if len(queued) != 1 {
		t.Fatalf("expected one queued message, got %d", len(queued))
	}
	if queued[0].Type != signaling.TypeSessionOffer {
		t.Fatalf("expected queued offer, got %s", queued[0].Type)
	}
}
