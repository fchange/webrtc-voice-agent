package bot

import (
	"sync"
	"time"
)

type endpointingCallbacks struct {
	onSpeechStart    func()
	onSpeechEnd      func()
	onEndOfUtterance func()
}

type packetEndpointer struct {
	mu           sync.Mutex
	active       bool
	closed       bool
	silenceTimer *time.Timer
	silenceAfter time.Duration
	callbacks    endpointingCallbacks
}

func newPacketEndpointer(silenceAfter time.Duration, callbacks endpointingCallbacks) *packetEndpointer {
	return &packetEndpointer{
		silenceAfter: silenceAfter,
		callbacks:    callbacks,
	}
}

func (e *packetEndpointer) ObservePacket() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}

	if !e.active {
		e.active = true
		if e.callbacks.onSpeechStart != nil {
			go e.callbacks.onSpeechStart()
		}
	}

	e.resetSilenceTimerLocked()
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

	if e.callbacks.onSpeechEnd != nil {
		e.callbacks.onSpeechEnd()
	}
	if e.callbacks.onEndOfUtterance != nil {
		e.callbacks.onEndOfUtterance()
	}
}
