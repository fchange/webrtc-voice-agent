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
	}, responseResult{Seq: 1, PGS: "apd"})
	acc.Apply(decodedResult{
		SN: 2,
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "好"}}},
		},
	}, responseResult{Seq: 2, PGS: "apd"})

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

func TestTranscriptAccumulatorPlainResultDoesNotAccumulateAcrossSN(t *testing.T) {
	acc := newTranscriptAccumulator()

	got := acc.Apply(decodedResult{
		SN: 1,
		RST: "pgs",
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "友"}}},
		},
	}, responseResult{Seq: 1})
	if got != "友" {
		t.Fatalf("expected 友, got %q", got)
	}

	got = acc.Apply(decodedResult{
		SN: 2,
		RST: "pgs",
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "友商"}}},
		},
	}, responseResult{Seq: 2})
	if got != "友商" {
		t.Fatalf("expected 友商, got %q", got)
	}

	got = acc.Apply(decodedResult{
		SN: 3,
		RST: "rlt",
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "友商是傻逼"}}},
		},
	}, responseResult{Seq: 3, Status: 1})
	if got != "友商是傻逼" {
		t.Fatalf("expected 友商是傻逼, got %q", got)
	}

	got = acc.Apply(decodedResult{
		SN: 4,
		LS:  true,
		RST: "rlt",
		WS: []decodedWordSlot{
			{CW: []decodedCandidateWord{{W: "。"}}},
		},
	}, responseResult{Seq: 4, Status: 2})
	if got != "友商是傻逼。" {
		t.Fatalf("expected 友商是傻逼。, got %q", got)
	}
}
