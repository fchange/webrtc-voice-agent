package datachannel

import "testing"

func TestEnvelopeValidate(t *testing.T) {
	envelope := Envelope{
		Version:   Version,
		Type:      TypeTurnInterrupt,
		SessionID: "sess_1",
		TurnID:    1,
	}

	if err := envelope.Validate(); err != nil {
		t.Fatalf("expected envelope to validate, got %v", err)
	}
}

func TestInterruptHintValidate(t *testing.T) {
	envelope := Envelope{
		Version:   Version,
		Type:      TypeTurnInterruptHint,
		SessionID: "sess_1",
		TurnID:    2,
		Payload: InterruptPayload{
			Reason: "local_vad_barge_in",
		},
	}

	if err := envelope.Validate(); err != nil {
		t.Fatalf("expected interrupt hint to validate, got %v", err)
	}
}

func TestASRFinalValidate(t *testing.T) {
	envelope := Envelope{
		Version:   Version,
		Type:      TypeASRFinal,
		SessionID: "sess_1",
		TurnID:    3,
		Payload: TranscriptPayload{
			Text:  "你好",
			Final: true,
		},
	}

	if err := envelope.Validate(); err != nil {
		t.Fatalf("expected asr final to validate, got %v", err)
	}
}

func TestEnvelopeValidateMissingSessionID(t *testing.T) {
	envelope := Envelope{
		Version: Version,
		Type:    TypeTurnInterrupt,
	}

	if err := envelope.Validate(); err == nil {
		t.Fatal("expected missing session_id to fail")
	}
}
