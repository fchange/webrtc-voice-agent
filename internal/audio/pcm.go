package audio

import (
	"encoding/binary"
	"fmt"
	"time"
)

func NormalizePCMFrame(frame PCMFrame, targetSampleRate uint32, targetChannels uint16) (PCMFrame, error) {
	if frame.SampleRate == 0 {
		return PCMFrame{}, fmt.Errorf("invalid sample rate")
	}
	if frame.Channels == 0 {
		return PCMFrame{}, fmt.Errorf("invalid channel count")
	}
	if targetChannels != 1 {
		return PCMFrame{}, fmt.Errorf("unsupported target channel count %d", targetChannels)
	}

	normalized := frame
	if normalized.Channels > 1 {
		normalized.Samples = downmixToMono(normalized.Samples, normalized.Channels)
		normalized.Channels = 1
	}

	if normalized.SampleRate != targetSampleRate {
		normalized.Samples = resampleMono(normalized.Samples, normalized.SampleRate, targetSampleRate)
		normalized.SampleRate = targetSampleRate
	}

	return normalized, nil
}

func PCMToS16LE(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func PCMFrameDuration(frame PCMFrame) time.Duration {
	if frame.SampleRate == 0 || frame.Channels == 0 || len(frame.Samples) == 0 {
		return 0
	}

	sampleCountPerChannel := len(frame.Samples) / int(frame.Channels)
	return time.Duration(sampleCountPerChannel) * time.Second / time.Duration(frame.SampleRate)
}

func downmixToMono(samples []int16, channels uint16) []int16 {
	if channels <= 1 {
		return append([]int16(nil), samples...)
	}

	channelCount := int(channels)
	if len(samples)%channelCount != 0 {
		return append([]int16(nil), samples...)
	}

	frameCount := len(samples) / channelCount
	out := make([]int16, frameCount)
	for i := 0; i < frameCount; i++ {
		sum := 0
		base := i * channelCount
		for ch := 0; ch < channelCount; ch++ {
			sum += int(samples[base+ch])
		}
		out[i] = int16(sum / channelCount)
	}
	return out
}

func resampleMono(samples []int16, srcRate uint32, dstRate uint32) []int16 {
	if srcRate == 0 || dstRate == 0 || len(samples) == 0 {
		return nil
	}
	if srcRate == dstRate {
		return append([]int16(nil), samples...)
	}

	outLen := int((int64(len(samples))*int64(dstRate) + int64(srcRate) - 1) / int64(srcRate))
	if outLen <= 0 {
		return nil
	}

	out := make([]int16, outLen)
	for i := 0; i < outLen; i++ {
		sourceIndex := int((int64(i) * int64(srcRate)) / int64(dstRate))
		if sourceIndex >= len(samples) {
			sourceIndex = len(samples) - 1
		}
		out[i] = samples[sourceIndex]
	}
	return out
}
