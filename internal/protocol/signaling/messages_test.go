package signaling

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestICECandidatePayloadMarshalsZeroLineIndex(t *testing.T) {
	index := uint16(0)
	payload := ICECandidatePayload{
		Candidate:     "candidate:1 1 udp 1234 127.0.0.1 123 typ host",
		SDPMLineIndex: &index,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if !strings.Contains(string(data), "\"sdp_mline_index\":0") {
		t.Fatalf("expected sdp_mline_index=0 to be preserved, got %s", string(data))
	}
}
