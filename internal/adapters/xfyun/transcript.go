package xfyun

import "strings"

type transcriptAccumulator struct {
	parts map[int]string
}

func newTranscriptAccumulator() *transcriptAccumulator {
	return &transcriptAccumulator{
		parts: make(map[int]string),
	}
}

func (a *transcriptAccumulator) Apply(decoded decodedResult, result responseResult) string {
	text := flattenDecodedText(decoded)

	switch result.PGS {
	case "rpl":
		if len(result.RG) == 2 {
			for i := result.RG[0]; i <= result.RG[1]; i++ {
				delete(a.parts, i)
			}
		}
		a.parts[decoded.SN] = text
	default:
		a.parts[decoded.SN] = text
	}

	maxSN := 0
	for sn := range a.parts {
		if sn > maxSN {
			maxSN = sn
		}
	}

	var builder strings.Builder
	for i := 1; i <= maxSN; i++ {
		builder.WriteString(a.parts[i])
	}
	return builder.String()
}

func flattenDecodedText(decoded decodedResult) string {
	var builder strings.Builder
	for _, slot := range decoded.WS {
		if len(slot.CW) == 0 {
			continue
		}
		builder.WriteString(slot.CW[0].W)
	}
	return builder.String()
}
