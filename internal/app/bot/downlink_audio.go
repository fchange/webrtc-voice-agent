package bot

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/audio"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	downlinkSampleRate    = 8000
	downlinkChannels      = 1
	downlinkFrameSamples  = 160
	downlinkQueueCapacity = 512
)

type queuedSample struct {
	data     []byte
	duration time.Duration
}

type downlinkAudioWriter struct {
	track     *webrtc.TrackLocalStaticSample
	logger    *slog.Logger
	pcmEndian string

	queue   chan queuedSample
	flushCh chan chan struct{}
	closeCh chan struct{}
	doneCh  chan struct{}
	once    sync.Once

	mu      sync.Mutex
	pending int
	idleCh  chan struct{}

	queuedSamples  int
	writtenSamples int
	writtenBytes   int
}

func newDownlinkAudioWriter(track *webrtc.TrackLocalStaticSample, logger *slog.Logger, pcmEndian string) *downlinkAudioWriter {
	writer := &downlinkAudioWriter{
		track:     track,
		logger:    logger,
		pcmEndian: pcmEndian,
		queue:     make(chan queuedSample, downlinkQueueCapacity),
		flushCh:   make(chan chan struct{}),
		closeCh:   make(chan struct{}),
		doneCh:    make(chan struct{}),
		idleCh:    closedSignal(),
	}
	go writer.run()
	return writer
}

func (w *downlinkAudioWriter) WritePCM16K(data []byte) error {
	if w == nil || w.track == nil || len(data) == 0 {
		return nil
	}
	if len(data)%2 != 0 {
		return fmt.Errorf("pcm payload must be 16-bit aligned")
	}

	samples, err := pcmBytesToInt16(data, w.pcmEndian)
	if err != nil {
		return err
	}
	frame, err := audio.NormalizePCMFrame(audio.PCMFrame{
		SampleRate: 16000,
		Channels:   1,
		Samples:    samples,
	}, downlinkSampleRate, downlinkChannels)
	if err != nil {
		return err
	}

	mulaw := pcm16ToMuLaw(frame.Samples)
	frames := splitMuLawFrames(mulaw, downlinkFrameSamples)
	if len(frames) == 0 {
		return nil
	}
	if w.logger != nil {
		w.logger.Info(
			"downlink pcm chunk normalized",
			"input_bytes", len(data),
			"samples", len(frame.Samples),
			"mulaw_bytes", len(mulaw),
			"frames", len(frames),
		)
	}
	for _, frame := range frames {
		w.markQueued(1)
		sample := queuedSample{
			data:     frame,
			duration: time.Duration(len(frame)) * time.Second / downlinkSampleRate,
		}
		if err := w.enqueue(sample); err != nil {
			w.markDone(1)
			return err
		}
	}

	return nil
}

func (w *downlinkAudioWriter) WaitIdle() {
	if w == nil {
		return
	}

	if w.logger != nil {
		w.logger.Info("downlink wait idle requested")
	}

	w.mu.Lock()
	idle := w.idleCh
	w.mu.Unlock()

	select {
	case <-idle:
	case <-w.doneCh:
	}
}

func (w *downlinkAudioWriter) Flush() {
	if w == nil {
		return
	}

	if w.logger != nil {
		w.logger.Info("downlink flush requested")
	}

	ack := make(chan struct{})
	select {
	case <-w.doneCh:
		return
	case w.flushCh <- ack:
	}

	select {
	case <-ack:
	case <-w.doneCh:
	}
}

func (w *downlinkAudioWriter) Close() {
	if w == nil {
		return
	}
	w.once.Do(func() {
		if w.logger != nil {
			w.logger.Info("downlink writer closing")
		}
		close(w.closeCh)
		<-w.doneCh
	})
}

func (w *downlinkAudioWriter) run() {
	defer func() {
		if w.logger != nil {
			w.mu.Lock()
			queuedSamples := w.queuedSamples
			writtenSamples := w.writtenSamples
			writtenBytes := w.writtenBytes
			pending := w.pending
			w.mu.Unlock()
			w.logger.Info(
				"downlink writer stopped",
				"queued_samples", queuedSamples,
				"written_samples", writtenSamples,
				"written_bytes", writtenBytes,
				"pending", pending,
			)
		}
		close(w.doneCh)
	}()

	for {
		select {
		case <-w.closeCh:
			return
		case ack := <-w.flushCh:
			w.flushQueue()
			close(ack)
		case sample := <-w.queue:
			if !w.writeAndPace(sample) {
				return
			}
		}
	}
}

func (w *downlinkAudioWriter) writeAndPace(sample queuedSample) bool {
	if err := w.track.WriteSample(media.Sample{
		Data:     sample.data,
		Duration: sample.duration,
	}); err != nil {
		if w.logger != nil {
			w.logger.Error("write downlink audio sample failed", "err", err, "bytes", len(sample.data), "duration", sample.duration.String())
		}
		w.markDone(1)
		return true
	}
	w.mu.Lock()
	w.writtenSamples++
	w.writtenBytes += len(sample.data)
	writtenSamples := w.writtenSamples
	writtenBytes := w.writtenBytes
	pending := w.pending
	w.mu.Unlock()
	if w.logger != nil && (writtenSamples == 1 || writtenSamples%50 == 0) {
		w.logger.Info(
			"downlink sample written",
			"sample_index", writtenSamples,
			"bytes", len(sample.data),
			"duration_ms", sample.duration.Milliseconds(),
			"pending", pending,
			"total_bytes", writtenBytes,
		)
	}

	timer := time.NewTimer(sample.duration)
	defer timer.Stop()

	select {
	case <-w.closeCh:
		return false
	case ack := <-w.flushCh:
		w.markDone(1)
		w.flushQueue()
		close(ack)
		return true
	case <-timer.C:
		w.markDone(1)
		return true
	}
}

func (w *downlinkAudioWriter) enqueue(sample queuedSample) error {
	select {
	case <-w.closeCh:
		return fmt.Errorf("downlink audio writer closed")
	case w.queue <- sample:
		w.mu.Lock()
		w.queuedSamples++
		queuedSamples := w.queuedSamples
		pending := w.pending
		w.mu.Unlock()
		if w.logger != nil && (queuedSamples == 1 || queuedSamples%50 == 0) {
			w.logger.Info(
				"downlink sample queued",
				"sample_index", queuedSamples,
				"bytes", len(sample.data),
				"duration_ms", sample.duration.Milliseconds(),
				"pending", pending,
			)
		}
		return nil
	}
}

func (w *downlinkAudioWriter) flushQueue() {
	drained := 0
	for {
		select {
		case <-w.queue:
			drained++
		default:
			if drained > 0 {
				if w.logger != nil {
					w.logger.Info("downlink queue drained", "samples", drained)
				}
				w.markDone(drained)
			}
			return
		}
	}
}

func (w *downlinkAudioWriter) markQueued(count int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if count <= 0 {
		return
	}
	if w.pending == 0 {
		w.idleCh = make(chan struct{})
	}
	w.pending += count
}

func (w *downlinkAudioWriter) markDone(count int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if count <= 0 || w.pending == 0 {
		return
	}
	w.pending -= count
	if w.pending <= 0 {
		w.pending = 0
		closeSignal(w.idleCh)
	}
}

func pcmBytesToInt16(data []byte, pcmEndian string) ([]int16, error) {
	samples := make([]int16, len(data)/2)
	var order binary.ByteOrder
	switch pcmEndian {
	case "", "big", "be":
		order = binary.BigEndian
	case "little", "le":
		order = binary.LittleEndian
	default:
		return nil, fmt.Errorf("unsupported pcm endian %q", pcmEndian)
	}
	for i := range samples {
		samples[i] = int16(order.Uint16(data[i*2:]))
	}
	return samples, nil
}

func pcm16ToMuLaw(samples []int16) []byte {
	out := make([]byte, len(samples))
	for i, sample := range samples {
		out[i] = linearToMuLaw(sample)
	}
	return out
}

func splitMuLawFrames(data []byte, frameSize int) [][]byte {
	if len(data) == 0 || frameSize <= 0 {
		return nil
	}

	frames := make([][]byte, 0, (len(data)+frameSize-1)/frameSize)
	for len(data) > 0 {
		size := frameSize
		if len(data) < size {
			size = len(data)
		}
		frame := append([]byte(nil), data[:size]...)
		frames = append(frames, frame)
		data = data[size:]
	}
	return frames
}

func closedSignal() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func closeSignal(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func linearToMuLaw(sample int16) byte {
	const (
		bias = 0x84
		clip = 32635
	)

	sign := byte(0)
	pcm := int(sample)
	if pcm < 0 {
		sign = 0x80
		pcm = -pcm
	}
	if pcm > clip {
		pcm = clip
	}
	pcm += bias

	exponent := 7
	mask := 0x4000
	for exponent > 0 && (pcm&mask) == 0 {
		exponent--
		mask >>= 1
	}
	mantissa := (pcm >> (exponent + 3)) & 0x0F

	return ^byte(int(sign) | (exponent << 4) | mantissa)
}
