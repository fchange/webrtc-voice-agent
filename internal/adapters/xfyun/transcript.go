package xfyun

import "strings"

type transcriptAccumulator struct {
	committed string
	parts     map[int]string
}

func newTranscriptAccumulator() *transcriptAccumulator {
	return &transcriptAccumulator{
		parts: make(map[int]string),
	}
}

func (a *transcriptAccumulator) Apply(decoded decodedResult, result responseResult) string {
	text := flattenDecodedText(decoded)

	var current string

	if result.PGS == "" {
		switch decoded.RST {
		case "pgs":
			a.parts = map[int]string{
				decoded.SN: text,
			}
			current = text
		case "rlt":
			a.parts = make(map[int]string)
			a.committed += text
			return a.committed
		default:
			a.parts = map[int]string{
				decoded.SN: text,
			}
			current = text
		}
	} else {
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
		current = assembleParts(a.parts)
	}

	return a.committed + current
}

func assembleParts(parts map[int]string) string {
	maxSN := 0
	for sn := range parts {
		if sn > maxSN {
			maxSN = sn
		}
	}

	var builder strings.Builder
	for i := 1; i <= maxSN; i++ {
		builder.WriteString(parts[i])
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
