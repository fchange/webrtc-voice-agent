package audio

import "testing"

func TestNormalizePCMFrameDownmixesAndResamples(t *testing.T) {
	frame := PCMFrame{
		SessionID:  "sess_1",
		TrackID:    "track_1",
		SampleRate: 48000,
		Channels:   2,
		Samples: []int16{
			10, 30,
			20, 40,
			30, 50,
			40, 60,
			50, 70,
			60, 80,
		},
	}

	normalized, err := NormalizePCMFrame(frame, 16000, 1)
	if err != nil {
		t.Fatalf("expected normalize to succeed, got %v", err)
	}

	if normalized.SampleRate != 16000 {
		t.Fatalf("expected 16000 Hz, got %d", normalized.SampleRate)
	}
	if normalized.Channels != 1 {
		t.Fatalf("expected mono, got %d channels", normalized.Channels)
	}

	expected := []int16{20, 50}
	if len(normalized.Samples) != len(expected) {
		t.Fatalf("expected %d samples, got %d", len(expected), len(normalized.Samples))
	}
	for i, sample := range expected {
		if normalized.Samples[i] != sample {
			t.Fatalf("expected sample[%d]=%d, got %d", i, sample, normalized.Samples[i])
		}
	}
}

func TestPCMToS16LE(t *testing.T) {
	got := PCMToS16LE([]int16{1, -2})
	expected := []byte{0x01, 0x00, 0xfe, 0xff}

	if len(got) != len(expected) {
		t.Fatalf("expected %d bytes, got %d", len(expected), len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("expected byte[%d]=%d, got %d", i, expected[i], got[i])
		}
	}
}
