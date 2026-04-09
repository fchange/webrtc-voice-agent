package volc

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	protoVersion = 0x1
	headerUnits  = 0x1 // 4 bytes
)

const (
	messageTypeFullClientRequest = 0x1
	messageTypeAudioOnlyRequest  = 0x2

	messageTypeFullServerResponse = 0x9
	messageTypeServerACK          = 0xB
	messageTypeServerError        = 0xF
)

const (
	flagNoSequence      = 0x0
	flagHasSequence     = 0x1
	flagIsLastPackage   = 0x2
	flagNegWithSequence = 0x3 // both bits set
)

const (
	serializationNone = 0x0
	serializationJSON = 0x1
)

const (
	compressionNone = 0x0
	compressionGZIP = 0x1
)

func makeHeader(messageType byte, flags byte, serialization byte, compression byte) [4]byte {
	var h [4]byte
	h[0] = byte((protoVersion << 4) | headerUnits)
	h[1] = byte((messageType << 4) | (flags & 0x0f))
	h[2] = byte(((serialization & 0x0f) << 4) | (compression & 0x0f))
	h[3] = 0
	return h
}

func appendI32BE(dst []byte, v int32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(v))
	return append(dst, buf[:]...)
}

func appendU32BE(dst []byte, v uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	return append(dst, buf[:]...)
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ungzipBytes(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type parsedPacket struct {
	MessageType byte
	Flags       byte
	Seq         *int32
	ErrCode     *uint32
	Payload     []byte
}

func parsePacket(data []byte) (parsedPacket, error) {
	if len(data) < 4 {
		return parsedPacket{}, fmt.Errorf("packet too short")
	}
	version := data[0] >> 4
	headerSizeUnits := data[0] & 0x0f
	if version != protoVersion {
		return parsedPacket{}, fmt.Errorf("unexpected protocol version=%d", version)
	}
	headerLen := int(headerSizeUnits) * 4
	if headerLen < 4 || len(data) < headerLen {
		return parsedPacket{}, fmt.Errorf("invalid header size=%d", headerSizeUnits)
	}

	messageType := data[1] >> 4
	flags := data[1] & 0x0f
	serialization := data[2] >> 4
	compression := data[2] & 0x0f

	offset := headerLen
	var seq *int32
	if flags&flagHasSequence != 0 {
		if len(data) < offset+4 {
			return parsedPacket{}, fmt.Errorf("missing sequence field")
		}
		v := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		seq = &v
		offset += 4
	}

	var errCode *uint32
	payload := []byte(nil)

	switch messageType {
	case messageTypeFullServerResponse:
		if len(data) < offset+4 {
			return parsedPacket{}, fmt.Errorf("missing payload size")
		}
		payloadSize := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if payloadSize < 0 || len(data) < offset+payloadSize {
			return parsedPacket{}, fmt.Errorf("payload truncated (size=%d)", payloadSize)
		}
		payload = data[offset : offset+payloadSize]
	case messageTypeServerACK:
		// ACK payload is optional; we don't need it for the adapter.
		// Some implementations include: ack_seq (4) + payload_size (4) + payload.
		payload = nil
	case messageTypeServerError:
		// Error response typically includes: code (4) + payload_size (4) + payload.
		if len(data) < offset+8 {
			return parsedPacket{}, fmt.Errorf("missing error fields")
		}
		code := binary.BigEndian.Uint32(data[offset : offset+4])
		errCode = &code
		offset += 4
		payloadSize := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if payloadSize < 0 || len(data) < offset+payloadSize {
			return parsedPacket{}, fmt.Errorf("error payload truncated (size=%d)", payloadSize)
		}
		payload = data[offset : offset+payloadSize]
	default:
		// Best-effort: treat it like a full response frame.
		if len(data) < offset+4 {
			return parsedPacket{}, fmt.Errorf("missing payload size")
		}
		payloadSize := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if payloadSize < 0 || len(data) < offset+payloadSize {
			return parsedPacket{}, fmt.Errorf("payload truncated (size=%d)", payloadSize)
		}
		payload = data[offset : offset+payloadSize]
	}

	if compression == compressionGZIP && len(payload) > 0 {
		decoded, err := ungzipBytes(payload)
		if err != nil {
			return parsedPacket{}, fmt.Errorf("gunzip payload: %w", err)
		}
		payload = decoded
	}
	_ = serialization // caller decides how to decode

	return parsedPacket{
		MessageType: messageType,
		Flags:       flags,
		Seq:         seq,
		ErrCode:     errCode,
		Payload:     payload,
	}, nil
}
