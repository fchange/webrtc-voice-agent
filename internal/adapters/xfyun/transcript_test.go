package xfyun

import "testing"

func TestTranscriptAccumulatorAppend(t *testing.T) {
	acc := newTranscriptAccumulator()
	got := acc.Apply(decodedResult{
		SN: 1,
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "你好"}}},
		},
	}, responseResult{Seq: 1})

	if got != "你好" {
		t.Fatalf("expected 你好, got %q", got)
	}
}

func TestTranscriptAccumulatorReplace(t *testing.T) {
	acc := newTranscriptAccumulator()
	acc.Apply(decodedResult{
		SN: 1,
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "你"}}},
		},
	}, responseResult{Seq: 1})
	acc.Apply(decodedResult{
		SN: 2,
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "好"}}},
		},
	}, responseResult{Seq: 2})

	got := acc.Apply(decodedResult{
		SN: 2,
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "世界"}}},
		},
	}, responseResult{
		Seq: 2,
		PGS: "rpl",
		RG:  []int{2, 2},
	})

	if got != "你世界" {
		t.Fatalf("expected 你世界, got %q", got)
	}
}
