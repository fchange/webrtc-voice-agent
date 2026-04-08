package audio

import "testing"

func TestEncodedPacketStreamPublishesPackets(t *testing.T) {
	stream := NewEncodedPacketStream()
	packets, cancel := stream.Subscribe(1)
	defer cancel()

	stream.Publish(EncodedPacket{
		SessionID:      "sess_1",
		SequenceNumber: 7,
		Payload:        []byte{1, 2, 3},
	})

	select {
	case packet := <-packets:
		if packet.SessionID != "sess_1" {
			t.Fatalf("expected sess_1, got %s", packet.SessionID)
		}
		if packet.SequenceNumber != 7 {
			t.Fatalf("expected seq 7, got %d", packet.SequenceNumber)
		}
	default:
		t.Fatal("expected published packet")
	}
}

func TestEncodedPacketStreamCloseClosesSubscribers(t *testing.T) {
	stream := NewEncodedPacketStream()
	packets, _ := stream.Subscribe(1)

	stream.Close()

	_, ok := <-packets
	if ok {
		t.Fatal("expected subscriber channel to be closed")
	}
}
