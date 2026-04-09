package bot

import (
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/audio"
)

type endpointingCallbacks struct {
	onSpeechStart    func()
	onSpeechEnd      func()
	onEndOfUtterance func()
}

type packetEndpointer struct {
	mu           sync.Mutex
	logger       *slog.Logger
	active       bool
	closed       bool
	silenceTimer *time.Timer
	silenceAfter time.Duration
	threshold    float64
	callbacks    endpointingCallbacks
}

func newPacketEndpointer(silenceAfter time.Duration, logger *slog.Logger, callbacks endpointingCallbacks) *packetEndpointer {
	if logger == nil {
		logger = slog.Default()
	}
	return &packetEndpointer{
		silenceAfter: silenceAfter,
		threshold:    700,
		logger:       logger,
		callbacks:    callbacks,
	}
}

func (e *packetEndpointer) ObserveFrame(frame audio.PCMFrame) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}

	rms := frameRMS(frame)
	if rms < e.threshold {
		if e.active {
			e.resetSilenceTimerLocked()
		}
		return
	}

	if !e.active {
		e.active = true
		e.logger.Info(
			"packet endpointing speech detected",
			"sample_rate", frame.SampleRate,
			"channels", frame.Channels,
			"samples", len(frame.Samples),
			"rms", rms,
			"threshold", e.threshold,
		)
		if e.callbacks.onSpeechStart != nil {
			go e.callbacks.onSpeechStart()
		}
	}

	if e.silenceTimer != nil {
		e.silenceTimer.Stop()
		e.silenceTimer = nil
		e.logger.Info(
			"packet endpointing speech resumed before silence timeout",
			"rms", rms,
			"threshold", e.threshold,
		)
	}
}

func (e *packetEndpointer) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.closed = true
	if e.silenceTimer != nil {
		e.silenceTimer.Stop()
		e.silenceTimer = nil
	}
}

func (e *packetEndpointer) resetSilenceTimerLocked() {
	if e.silenceTimer == nil {
		e.logger.Info(
			"packet endpointing silence window started",
			"silence_after_ms", e.silenceAfter.Milliseconds(),
		)
		e.silenceTimer = time.AfterFunc(e.silenceAfter, e.handleSilenceTimeout)
		return
	}

	if !e.silenceTimer.Stop() {
		return
	}
	e.silenceTimer.Reset(e.silenceAfter)
}

func (e *packetEndpointer) handleSilenceTimeout() {
	e.mu.Lock()
	if e.closed || !e.active {
		e.mu.Unlock()
		return
	}
	e.active = false
	e.mu.Unlock()

	e.logger.Info("packet endpointing silence timeout fired", "silence_after_ms", e.silenceAfter.Milliseconds())
	if e.callbacks.onSpeechEnd != nil {
		e.callbacks.onSpeechEnd()
	}
	if e.callbacks.onEndOfUtterance != nil {
		e.callbacks.onEndOfUtterance()
	}
}

func frameHasSpeech(frame audio.PCMFrame, threshold float64) bool {
	return frameRMS(frame) >= threshold
}

func frameRMS(frame audio.PCMFrame) float64 {
	if len(frame.Samples) == 0 {
		return 0
	}

	var sum float64
	for _, sample := range frame.Samples {
		value := float64(sample)
		sum += value * value
	}
	return math.Sqrt(sum / float64(len(frame.Samples)))
}
