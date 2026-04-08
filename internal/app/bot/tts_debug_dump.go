package bot

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ttsDebugDumper struct {
	dir       string
	pcmEndian string
}

func newTTSDebugDumper(dir string, pcmEndian string) *ttsDebugDumper {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	return &ttsDebugDumper{
		dir:       dir,
		pcmEndian: pcmEndian,
	}
}

func (d *ttsDebugDumper) Dump(sessionID string, turnID int64, segmentID int, text string, raw []byte) error {
	if d == nil || len(raw) == 0 {
		return nil
	}

	sessionDir := filepath.Join(d.dir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return err
	}

	baseName := fmt.Sprintf("turn_%02d_segment_%02d_%s", turnID, segmentID, sanitizeFilename(text))
	rawPath := filepath.Join(sessionDir, baseName+".raw")
	wavPath := filepath.Join(sessionDir, baseName+".wav")
	txtPath := filepath.Join(sessionDir, baseName+".txt")

	if err := os.WriteFile(rawPath, raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(txtPath, []byte(text), 0o644); err != nil {
		return err
	}

	if samples, err := pcmBytesToInt16(raw, d.pcmEndian); err == nil {
		if err := writePCM16WAV(wavPath, 16000, 1, samples); err != nil {
			return err
		}
	}

	variants := []struct {
		endian     string
		sampleRate uint32
		label      string
	}{
		{endian: "big", sampleRate: 16000, label: "be_16k"},
		{endian: "little", sampleRate: 16000, label: "le_16k"},
		{endian: "big", sampleRate: 8000, label: "be_8k"},
		{endian: "little", sampleRate: 8000, label: "le_8k"},
	}

	for _, variant := range variants {
		samples, err := pcmBytesToInt16(raw, variant.endian)
		if err != nil {
			return err
		}
		path := filepath.Join(sessionDir, baseName+"."+variant.label+".wav")
		if err := writePCM16WAV(path, variant.sampleRate, 1, samples); err != nil {
			return err
		}
	}

	return nil
}

func writePCM16WAV(path string, sampleRate uint32, channels uint16, samples []int16) error {
	dataSize := uint32(len(samples) * 2)
	fileSize := 36 + dataSize

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writeString := func(value string) error {
		_, err := file.WriteString(value)
		return err
	}
	writeU16 := func(value uint16) error {
		return binary.Write(file, binary.LittleEndian, value)
	}
	writeU32 := func(value uint32) error {
		return binary.Write(file, binary.LittleEndian, value)
	}

	if err := writeString("RIFF"); err != nil {
		return err
	}
	if err := writeU32(fileSize); err != nil {
		return err
	}
	if err := writeString("WAVE"); err != nil {
		return err
	}
	if err := writeString("fmt "); err != nil {
		return err
	}
	if err := writeU32(16); err != nil {
		return err
	}
	if err := writeU16(1); err != nil {
		return err
	}
	if err := writeU16(channels); err != nil {
		return err
	}
	if err := writeU32(sampleRate); err != nil {
		return err
	}
	byteRate := sampleRate * uint32(channels) * 2
	if err := writeU32(byteRate); err != nil {
		return err
	}
	blockAlign := channels * 2
	if err := writeU16(blockAlign); err != nil {
		return err
	}
	if err := writeU16(16); err != nil {
		return err
	}
	if err := writeString("data"); err != nil {
		return err
	}
	if err := writeU32(dataSize); err != nil {
		return err
	}

	for _, sample := range samples {
		if err := binary.Write(file, binary.LittleEndian, sample); err != nil {
			return err
		}
	}
	return nil
}

var filenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_\p{Han}]+`)

func sanitizeFilename(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "empty"
	}
	sanitized := filenameSanitizer.ReplaceAllString(trimmed, "_")
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		return "segment"
	}
	if len([]rune(sanitized)) > 24 {
		sanitized = string([]rune(sanitized)[:24])
	}
	return sanitized
}
