package opus

import (
	"testing"

	"github.com/fchange/webrtc-voice-agent/internal/audio"
)

func TestFactoryDefaultsToBrowserOpusLayout(t *testing.T) {
	decoder, err := NewFactory().New(audio.EncodedPacket{
		Codec: audio.CodecOpus,
	})
	if err != nil {
		t.Fatalf("expected decoder creation to succeed, got %v", err)
	}

	opusDecoder, ok := decoder.(*Decoder)
	if !ok {
		t.Fatalf("expected *Decoder, got %T", decoder)
	}
	if opusDecoder.sampleRate != defaultOpusSampleRate {
		t.Fatalf("expected sample rate %d, got %d", defaultOpusSampleRate, opusDecoder.sampleRate)
	}
	if opusDecoder.channels != defaultOpusChannels {
		t.Fatalf("expected channels %d, got %d", defaultOpusChannels, opusDecoder.channels)
	}
}

func TestFactoryUsesPacketMetadataWhenPresent(t *testing.T) {
	decoder, err := NewFactory().New(audio.EncodedPacket{
		Codec:     audio.CodecOpus,
		ClockRate: 48000,
		Channels:  1,
	})
	if err != nil {
		t.Fatalf("expected decoder creation to succeed, got %v", err)
	}

	opusDecoder := decoder.(*Decoder)
	if opusDecoder.channels != 1 {
		t.Fatalf("expected mono decoder, got %d channels", opusDecoder.channels)
	}
}
