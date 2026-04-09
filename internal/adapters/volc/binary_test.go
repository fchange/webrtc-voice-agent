package volc

import (
	"encoding/json"
	"testing"
)

func TestParsePacketFullServerResponse(t *testing.T) {
	payloadMap := map[string]any{
		"result": map[string]any{
			"text": "你好，世界",
		},
	}
	payloadJSON, err := json.Marshal(payloadMap)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	compressed, err := gzipBytes(payloadJSON)
	if err != nil {
		t.Fatalf("gzip payload: %v", err)
	}

	packet := make([]byte, 0, 4+4+4+len(compressed))
	header := makeHeader(messageTypeFullServerResponse, flagNegWithSequence, serializationJSON, compressionGZIP)
	packet = append(packet, header[:]...)
	packet = appendI32BE(packet, -3)
	packet = appendU32BE(packet, uint32(len(compressed)))
	packet = append(packet, compressed...)

	parsed, err := parsePacket(packet)
	if err != nil {
		t.Fatalf("parse packet: %v", err)
	}
	if parsed.MessageType != messageTypeFullServerResponse {
		t.Fatalf("expected full server response, got %d", parsed.MessageType)
	}
	if parsed.Seq == nil || *parsed.Seq != -3 {
		t.Fatalf("expected seq -3, got %+v", parsed.Seq)
	}
	if parsed.Flags != flagNegWithSequence {
		t.Fatalf("expected neg-with-sequence flag, got %d", parsed.Flags)
	}
	if string(parsed.Payload) != string(payloadJSON) {
		t.Fatalf("expected payload %s, got %s", string(payloadJSON), string(parsed.Payload))
	}
}

func TestParsePacketServerError(t *testing.T) {
	body := []byte(`{"error":"requested resource not granted"}`)

	packet := make([]byte, 0, 4+4+4+len(body))
	header := makeHeader(messageTypeServerError, flagNoSequence, serializationJSON, compressionNone)
	packet = append(packet, header[:]...)
	packet = appendU32BE(packet, 403)
	packet = appendU32BE(packet, uint32(len(body)))
	packet = append(packet, body...)

	parsed, err := parsePacket(packet)
	if err != nil {
		t.Fatalf("parse packet: %v", err)
	}
	if parsed.MessageType != messageTypeServerError {
		t.Fatalf("expected server error type, got %d", parsed.MessageType)
	}
	if parsed.ErrCode == nil || *parsed.ErrCode != 403 {
		t.Fatalf("expected error code 403, got %+v", parsed.ErrCode)
	}
	if string(parsed.Payload) != string(body) {
		t.Fatalf("expected payload %s, got %s", string(body), string(parsed.Payload))
	}
}

