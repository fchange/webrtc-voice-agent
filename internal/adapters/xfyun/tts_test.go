package xfyun

import "testing"

func TestMarshalTTSTextBase64(t *testing.T) {
	got := MarshalTTSTextBase64("你好")
	if got == "" {
		t.Fatal("expected non-empty base64 text")
	}
}

func TestDecodeTTSAudioBase64(t *testing.T) {
	audio, err := DecodeTTSAudioBase64("aGVsbG8=")
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if string(audio) != "hello" {
		t.Fatalf("expected hello, got %q", string(audio))
	}
}
