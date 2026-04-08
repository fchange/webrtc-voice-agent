package opus

import (
	"fmt"

	godepsopus "github.com/godeps/opus"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/audio"
)

const (
	defaultOpusSampleRate = 48000
	defaultOpusChannels   = 2
	maxSamplesPerChannel  = 5760
)

type Factory struct{}

func NewFactory() Factory {
	return Factory{}
}

func (Factory) Codec() audio.Codec {
	return audio.CodecOpus
}

func (Factory) New(packet audio.EncodedPacket) (audio.Decoder, error) {
	sampleRate := int(packet.ClockRate)
	if sampleRate == 0 {
		sampleRate = defaultOpusSampleRate
	}

	channels := int(packet.Channels)
	if channels == 0 {
		channels = defaultOpusChannels
	}

	decoder, err := godepsopus.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, fmt.Errorf("create opus decoder: %w", err)
	}

	return &Decoder{
		decoder:    decoder,
		sampleRate: uint32(sampleRate),
		channels:   uint16(channels),
		buffer:     make([]int16, maxSamplesPerChannel*channels),
	}, nil
}

type Decoder struct {
	decoder    *godepsopus.Decoder
	sampleRate uint32
	channels   uint16
	buffer     []int16
}

func (d *Decoder) Decode(packet audio.EncodedPacket) ([]audio.PCMFrame, error) {
	if d.decoder == nil {
		return nil, fmt.Errorf("opus decoder is not initialized")
	}

	samplesPerChannel, err := d.decoder.Decode(packet.Payload, d.buffer)
	if err != nil {
		return nil, fmt.Errorf("decode opus packet: %w", err)
	}

	totalSamples := samplesPerChannel * int(d.channels)
	if totalSamples > len(d.buffer) {
		return nil, fmt.Errorf("decoded opus frame exceeds buffer: %d > %d", totalSamples, len(d.buffer))
	}

	samples := append([]int16(nil), d.buffer[:totalSamples]...)

	return []audio.PCMFrame{
		{
			SessionID:  packet.SessionID,
			TrackID:    packet.TrackID,
			SampleRate: d.sampleRate,
			Channels:   d.channels,
			Samples:    samples,
			ReceivedAt: packet.ReceivedAt,
		},
	}, nil
}
