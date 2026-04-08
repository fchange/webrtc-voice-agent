package bot

import "testing"

func TestPCM16KToMuLawKeepsExpectedLength(t *testing.T) {
	data := make([]byte, 320*2) // 20ms @ 16k mono s16le
	samples, err := pcmBytesToInt16(data, "little")
	if err != nil {
		t.Fatalf("pcmBytesToInt16 failed: %v", err)
	}
	if got := len(samples); got != 320 {
		t.Fatalf("expected 320 pcm samples, got %d", got)
	}

	writer := newDownlinkAudioWriter(nil, nil, "little")
	if err := writer.WritePCM16K(data); err != nil {
		t.Fatalf("expected nil writer to be no-op, got %v", err)
	}
	writer.Close()

	samples8k := pcm16ToMuLaw(make([]int16, 160))
	if len(samples8k) != 160 {
		t.Fatalf("expected 160 mulaw bytes, got %d", len(samples8k))
	}
}

func TestSplitMuLawFramesUses20msPackets(t *testing.T) {
	data := make([]byte, 401)
	frames := splitMuLawFrames(data, downlinkFrameSamples)

	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	if got := len(frames[0]); got != downlinkFrameSamples {
		t.Fatalf("expected first frame size %d, got %d", downlinkFrameSamples, got)
	}
	if got := len(frames[1]); got != downlinkFrameSamples {
		t.Fatalf("expected second frame size %d, got %d", downlinkFrameSamples, got)
	}
	if got := len(frames[2]); got != 81 {
		t.Fatalf("expected tail frame size 81, got %d", got)
	}
}
