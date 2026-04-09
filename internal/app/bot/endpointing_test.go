package bot

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/audio"
)

func TestFrameHasSpeech(t *testing.T) {
	silent := audio.PCMFrame{
		SampleRate: 16000,
		Channels:   1,
		Samples:    []int16{0, 0, 10, -10, 0},
	}
	if frameHasSpeech(silent, 700) {
		t.Fatal("expected silent frame to be treated as silence")
	}

	speech := audio.PCMFrame{
		SampleRate: 16000,
		Channels:   1,
		Samples:    []int16{2000, -1800, 1900, -1700},
	}
	if !frameHasSpeech(speech, 700) {
		t.Fatal("expected speech frame to be treated as speech")
	}
}

func TestPacketEndpointerDetectsSpeechThenSilenceFromPCM(t *testing.T) {
	var mu sync.Mutex
	speechStarted := 0
	speechEnded := 0
	utterancesEnded := 0

	endpointer := newPacketEndpointer(20*time.Millisecond, slog.Default(), endpointingCallbacks{
		onSpeechStart: func() {
			mu.Lock()
			speechStarted++
			mu.Unlock()
		},
		onSpeechEnd: func() {
			mu.Lock()
			speechEnded++
			mu.Unlock()
		},
		onEndOfUtterance: func() {
			mu.Lock()
			utterancesEnded++
			mu.Unlock()
		},
	})
	defer endpointer.Close()

	endpointer.ObserveFrame(audio.PCMFrame{
		SampleRate: 16000,
		Channels:   1,
		Samples:    []int16{2000, -1800, 1700, -1500},
	})

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		started := speechStarted
		mu.Unlock()
		if started == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	for i := 0; i < 3; i++ {
		endpointer.ObserveFrame(audio.PCMFrame{
			SampleRate: 16000,
			Channels:   1,
			Samples:    []int16{0, 0, 0, 0},
		})
		time.Sleep(10 * time.Millisecond)
	}

	deadline = time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		started := speechStarted
		ended := speechEnded
		utterEnded := utterancesEnded
		mu.Unlock()
		if started == 1 && ended == 1 && utterEnded == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("expected start/end/eou callbacks once, got start=%d end=%d eou=%d", speechStarted, speechEnded, utterancesEnded)
}
