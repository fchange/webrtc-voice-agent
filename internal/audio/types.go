package audio

import (
	"strings"
	"time"
)

type Codec string

const (
	CodecUnknown Codec = "unknown"
	CodecOpus    Codec = "audio/opus"
)

func CodecFromMimeType(mimeType string) Codec {
	switch strings.ToLower(mimeType) {
	case string(CodecOpus):
		return CodecOpus
	default:
		if mimeType == "" {
			return CodecUnknown
		}
		return Codec(strings.ToLower(mimeType))
	}
}

type EncodedPacket struct {
	SessionID      string
	TrackID        string
	StreamID       string
	Codec          Codec
	CodecMimeType  string
	ClockRate      uint32
	Channels       uint16
	PayloadType    uint8
	SequenceNumber uint16
	RTPTime        uint32
	Marker         bool
	ReceivedAt     time.Time
	Payload        []byte
}

type PCMFrame struct {
	SessionID  string
	TrackID    string
	SampleRate uint32
	Channels   uint16
	Samples    []int16
	ReceivedAt time.Time
}
