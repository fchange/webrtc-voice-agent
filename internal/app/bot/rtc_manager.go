package bot

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	"github.com/fchange/webrtc-voice-agent/internal/audio"
	opusaudio "github.com/fchange/webrtc-voice-agent/internal/audio/opus"
	"github.com/fchange/webrtc-voice-agent/internal/config"
	"github.com/fchange/webrtc-voice-agent/internal/protocol/signaling"
	"github.com/fchange/webrtc-voice-agent/internal/session"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type signalWriter interface {
	WriteEnvelope(signaling.Envelope) error
}

type rtcSession struct {
	pc                 *webrtc.PeerConnection
	pendingICE         []webrtc.ICECandidateInit
	signal             signalWriter
	remoteSet          bool
	controlReady       bool
	endingScheduled    bool
	openingTurnStarted bool
	endpointer         *packetEndpointer
	upstream           *audio.EncodedPacketStream
	pcm                *audio.PCMFrameStream
	asr                *asrRuntime
	response           *responseRuntime
	botAudio           *webrtc.TrackLocalStaticSample
	downlink           *downlinkAudioWriter
}

type rtcManager struct {
	mu              sync.Mutex
	cfg             webrtc.Configuration
	logger          *slog.Logger
	session         map[string]*rtcSession
	control         *controlRuntime
	manager         *session.Manager
	asrProvider     adapters.ASRAdapter
	llmProvider     adapters.LLMAdapter
	ttsProvider     adapters.TTSAdapter
	asrSampleRate   uint32
	endpointSilence time.Duration
	segmenter       config.LLMSegmenterConfig
	ttsConfig       config.XFYUNTTSConfig
	decoderRegistry *audio.Registry
}

const sessionEndingGracePeriod = 700 * time.Millisecond

func newRTCManager(
	stunURL string,
	logger *slog.Logger,
	manager *session.Manager,
	control *controlRuntime,
	asrProvider adapters.ASRAdapter,
	llmProvider adapters.LLMAdapter,
	ttsProvider adapters.TTSAdapter,
	asrSampleRate uint32,
	endpointSilence time.Duration,
	segmenter config.LLMSegmenterConfig,
	ttsConfig config.XFYUNTTSConfig,
) *rtcManager {
	cfg := webrtc.Configuration{}
	if stunURL != "" {
		cfg.ICEServers = []webrtc.ICEServer{
			{URLs: []string{stunURL}},
		}
	}

	registry := audio.NewRegistry()
	registry.Register(opusaudio.NewFactory())
	if endpointSilence <= 0 {
		endpointSilence = 900 * time.Millisecond
	}

	return &rtcManager{
		cfg:             cfg,
		logger:          logger,
		session:         make(map[string]*rtcSession),
		control:         control,
		manager:         manager,
		asrProvider:     asrProvider,
		llmProvider:     llmProvider,
		ttsProvider:     ttsProvider,
		asrSampleRate:   asrSampleRate,
		endpointSilence: endpointSilence,
		segmenter:       segmenter,
		ttsConfig:       ttsConfig,
		decoderRegistry: registry,
	}
}

func (m *rtcManager) markControlReady(sessionID string) {
	m.mu.Lock()
	state := m.session[sessionID]
	if state == nil {
		state = &rtcSession{}
		m.session[sessionID] = state
	}
	state.controlReady = true
	m.mu.Unlock()

	m.maybeStartOpeningTurn(sessionID)
}

func (m *rtcManager) maybeStartOpeningTurn(sessionID string) {
	m.mu.Lock()
	state := m.session[sessionID]
	if state == nil || !state.controlReady || state.response == nil || state.openingTurnStarted || state.endingScheduled {
		m.mu.Unlock()
		return
	}
	response := state.response
	state.openingTurnStarted = true
	m.mu.Unlock()

	if err := response.StartAssistantTurn(openingGreetingText); err != nil {
		m.logger.Info("skip opening greeting", "session_id", sessionID, "err", err)
		m.mu.Lock()
		if current := m.session[sessionID]; current != nil {
			current.openingTurnStarted = false
		}
		m.mu.Unlock()
	}
}

func (m *rtcManager) ScheduleEnd(sessionID string, reason string, message string) {
	m.mu.Lock()
	state := m.session[sessionID]
	if state == nil || state.endingScheduled {
		m.mu.Unlock()
		return
	}
	state.endingScheduled = true
	signal := state.signal
	m.mu.Unlock()

	if m.control != nil {
		m.control.emitSessionEnding(sessionID, message)
	}

	go func() {
		time.Sleep(sessionEndingGracePeriod)
		if signal != nil {
			if err := signal.WriteEnvelope(signaling.Envelope{
				Version:   signaling.Version,
				Type:      signaling.TypeSessionClose,
				SessionID: sessionID,
				Payload: mustMarshal(map[string]string{
					"reason": reason,
				}),
			}); err != nil {
				m.logger.Info("send session close to signal failed", "session_id", sessionID, "err", err)
			}
		}
		m.Close(sessionID)
		_ = m.manager.End(sessionID)
	}()
}

func (m *rtcManager) isEnding(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.session[sessionID]
	return state != nil && state.endingScheduled
}

func (m *rtcManager) HandleOffer(sessionID string, payload signaling.SDPPayload, writer signalWriter) error {
	m.mu.Lock()
	existing := m.session[sessionID]
	if existing == nil {
		existing = &rtcSession{signal: writer}
		m.session[sessionID] = existing
	} else {
		existing.signal = writer
	}
	m.mu.Unlock()

	if existing.pc == nil {
		pc, err := m.newPeerConnection(sessionID, writer)
		if err != nil {
			return err
		}

		m.mu.Lock()
		existing.pc = pc
		m.mu.Unlock()
	}

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  payload.SDP,
	}
	if err := existing.pc.SetRemoteDescription(offer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	m.mu.Lock()
	existing.remoteSet = true
	pending := append([]webrtc.ICECandidateInit(nil), existing.pendingICE...)
	existing.pendingICE = nil
	m.mu.Unlock()

	for _, candidate := range pending {
		if err := existing.pc.AddICECandidate(candidate); err != nil {
			return fmt.Errorf("flush pending ice: %w", err)
		}
	}

	answer, err := existing.pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}
	if err := existing.pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	return writer.WriteEnvelope(signaling.Envelope{
		Version:   signaling.Version,
		Type:      signaling.TypeSessionAnswer,
		SessionID: sessionID,
		Payload: mustMarshal(signaling.SDPPayload{
			SDP:  answer.SDP,
			Type: answer.Type.String(),
		}),
	})
}

func (m *rtcManager) HandleICECandidate(sessionID string, payload signaling.ICECandidatePayload) error {
	init := webrtc.ICECandidateInit{
		Candidate: payload.Candidate,
	}
	if payload.SDPMid != "" {
		mid := payload.SDPMid
		init.SDPMid = &mid
	}
	if payload.SDPMLineIndex != nil {
		index := *payload.SDPMLineIndex
		init.SDPMLineIndex = &index
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.session[sessionID]
	if !ok || state.pc == nil || !state.remoteSet {
		if !ok {
			state = &rtcSession{}
			m.session[sessionID] = state
		}
		state.pendingICE = append(state.pendingICE, init)
		return nil
	}

	return state.pc.AddICECandidate(init)
}

func (m *rtcManager) Close(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.session[sessionID]
	if !ok {
		return
	}
	delete(m.session, sessionID)
	if state.pc != nil {
		_ = state.pc.Close()
	}
	if state.upstream != nil {
		state.upstream.Close()
	}
	if state.pcm != nil {
		state.pcm.Close()
	}
	if state.asr != nil {
		state.asr.Close()
	}
	if state.response != nil {
		state.response.Close()
	}
	if state.downlink != nil {
		state.downlink.Close()
	}
	if closer, ok := state.signal.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func (m *rtcManager) newPeerConnection(sessionID string, writer signalWriter) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(m.cfg)
	if err != nil {
		return nil, fmt.Errorf("new peer connection: %w", err)
	}

	botAudioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  1,
		},
		"bot-audio",
		sessionID,
	)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("create bot audio track: %w", err)
	}
	rtpSender, err := pc.AddTrack(botAudioTrack)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("add bot audio track: %w", err)
	}
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, readErr := rtpSender.Read(rtcpBuf); readErr != nil {
				return
			}
		}
	}()

	m.mu.Lock()
	state := m.session[sessionID]
	if state != nil {
		state.botAudio = botAudioTrack
	}
	m.mu.Unlock()

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		jsonCandidate := candidate.ToJSON()

		if err := writer.WriteEnvelope(signaling.Envelope{
			Version:   signaling.Version,
			Type:      signaling.TypeSessionICE,
			SessionID: sessionID,
			Payload: mustMarshal(signaling.ICECandidatePayload{
				Candidate:     jsonCandidate.Candidate,
				SDPMid:        stringPtrValue(jsonCandidate.SDPMid),
				SDPMLineIndex: jsonCandidate.SDPMLineIndex,
			}),
		}); err != nil {
			m.logger.Error("send local ice candidate failed", "session_id", sessionID, "err", err)
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		m.logger.Info("peer connection state changed", "session_id", sessionID, "state", state.String())
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			m.Close(sessionID)
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		m.logger.Info(
			"received remote track",
			"session_id", sessionID,
			"kind", track.Kind().String(),
			"codec", track.Codec().MimeType,
			"ssrc", track.SSRC(),
		)
		_ = receiver
		upstream := audio.NewEncodedPacketStream()
		pcmStream := audio.NewPCMFrameStream()
		downlink := newDownlinkAudioWriter(botAudioTrack, m.logger, m.ttsConfig.PCMEndian)
		responseRuntime := newResponseRuntime(
			sessionID,
			m.manager,
			m.llmProvider,
			m.ttsProvider,
			m.control,
			m,
			m.segmenter,
			downlink,
			newTTSDebugDumper(m.ttsConfig.DebugDumpDir, m.ttsConfig.PCMEndian),
			m.logger,
		)
		endpointer := newPacketEndpointer(m.endpointSilence, m.logger.With("session_id", sessionID), endpointingCallbacks{
			onSpeechStart: func() {
				if m.isEnding(sessionID) {
					return
				}
				if m.control != nil {
					m.control.handleVADStart(sessionID)
				}
				if runtime := m.asrRuntime(sessionID); runtime != nil {
					runtime.HandleSpeechStart()
				}
			},
			onSpeechEnd: func() {
				if m.control != nil {
					m.control.handleVADEnd(sessionID)
				}
			},
			onEndOfUtterance: func() {
				if runtime := m.asrRuntime(sessionID); runtime != nil {
					runtime.HandleEndOfUtterance()
				}
				if m.control != nil {
					m.control.handleEndOfUtterance(sessionID)
				}
			},
		})
		asrRuntime := newASRRuntime(sessionID, m.manager, m.asrProvider, m.control, responseRuntime, m.asrSampleRate, m.logger)

		m.mu.Lock()
		state := m.session[sessionID]
		if state != nil {
			state.endpointer = endpointer
			state.upstream = upstream
			state.pcm = pcmStream
			state.asr = asrRuntime
			state.response = responseRuntime
			state.downlink = downlink
		}
		m.mu.Unlock()
		m.maybeStartOpeningTurn(sessionID)
		m.attachDecodeConsumer(sessionID, upstream, pcmStream)
		m.attachEndpointingConsumer(sessionID, pcmStream, endpointer)
		asrRuntime.Attach(pcmStream)
		defer upstream.Close()
		defer pcmStream.Close()
		defer endpointer.Close()
		defer asrRuntime.Close()
		defer responseRuntime.Close()
		defer downlink.Close()

		for {
			packet, _, err := track.ReadRTP()
			if err != nil {
				m.logger.Info("remote track ended", "session_id", sessionID, "err", err)
				return
			}
			upstream.Publish(newEncodedAudioPacket(sessionID, track, packet))
		}
	})

	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		m.logger.Info("data channel opened by remote", "session_id", sessionID, "label", channel.Label())
		if channel.Label() != "control" {
			channel.OnMessage(func(msg webrtc.DataChannelMessage) {
				m.logger.Info("non-control data channel message", "session_id", sessionID, "label", channel.Label(), "bytes", len(msg.Data))
			})
			return
		}

		if m.control != nil {
			m.control.bind(sessionID, channel)
		}
	})

	return pc, nil
}

func (m *rtcManager) attachEndpointingConsumer(sessionID string, pcmStream *audio.PCMFrameStream, endpointer *packetEndpointer) {
	frames, cancel := pcmStream.Subscribe(32)
	go func() {
		defer cancel()
		for frame := range frames {
			endpointer.ObserveFrame(frame)
		}
		m.logger.Info("endpointing consumer closed", "session_id", sessionID)
	}()
}

func (m *rtcManager) attachDecodeConsumer(sessionID string, upstream *audio.EncodedPacketStream, pcmStream *audio.PCMFrameStream) {
	packets, cancel := upstream.Subscribe(64)
	go func() {
		defer cancel()
		var decoder audio.Decoder
		decodedPackets := 0
		publishedFrames := 0
		for packet := range packets {
			if decoder == nil {
				var err error
				decoder, err = m.decoderRegistry.NewDecoder(packet)
				if err != nil {
					m.logger.Warn("create audio decoder failed", "session_id", sessionID, "codec", packet.Codec, "err", err)
					continue
				}
				m.logger.Info(
					"audio decoder created",
					"session_id", sessionID,
					"codec", packet.Codec,
					"mime_type", packet.CodecMimeType,
					"clock_rate", packet.ClockRate,
					"channels", packet.Channels,
				)
			}

			frames, err := decoder.Decode(packet)
			if err != nil {
				m.logger.Warn("decode upstream audio packet failed", "session_id", sessionID, "codec", packet.Codec, "err", err)
				continue
			}
			decodedPackets++
			if decodedPackets == 1 {
				m.logger.Info(
					"first upstream audio packet decoded",
					"session_id", sessionID,
					"codec", packet.Codec,
					"sequence_number", packet.SequenceNumber,
					"payload_bytes", len(packet.Payload),
					"frames", len(frames),
				)
			}
			for _, frame := range frames {
				pcmStream.Publish(frame)
				publishedFrames++
				if publishedFrames == 1 {
					m.logger.Info(
						"first pcm frame published",
						"session_id", sessionID,
						"sample_rate", frame.SampleRate,
						"channels", frame.Channels,
						"samples", len(frame.Samples),
						"duration_ms", audio.PCMFrameDuration(frame).Milliseconds(),
					)
				}
			}
		}
		m.logger.Info("decode consumer closed", "session_id", sessionID, "decoded_packets", decodedPackets, "published_frames", publishedFrames)
	}()
}

func (m *rtcManager) asrRuntime(sessionID string) *asrRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.session[sessionID]
	if state == nil {
		return nil
	}
	return state.asr
}

func (m *rtcManager) interruptResponse(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.session[sessionID]
	if state == nil || state.response == nil {
		return
	}
	state.response.Interrupt()
}

func newEncodedAudioPacket(sessionID string, track *webrtc.TrackRemote, packet *rtp.Packet) audio.EncodedPacket {
	payload := append([]byte(nil), packet.Payload...)
	codec := track.Codec()
	return audio.EncodedPacket{
		SessionID:      sessionID,
		TrackID:        track.ID(),
		StreamID:       track.StreamID(),
		Codec:          audio.CodecFromMimeType(codec.MimeType),
		CodecMimeType:  codec.MimeType,
		ClockRate:      codec.ClockRate,
		Channels:       codec.Channels,
		PayloadType:    packet.PayloadType,
		SequenceNumber: packet.SequenceNumber,
		RTPTime:        packet.Timestamp,
		Marker:         packet.Marker,
		ReceivedAt:     time.Now().UTC(),
		Payload:        payload,
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
