package volc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

type ASR struct {
	cfg    config.VolcASRConfig
	dialer *websocket.Dialer
	logger *slog.Logger
}

func NewASR(cfg config.VolcASRConfig, logger *slog.Logger) *ASR {
	if logger == nil {
		logger = slog.Default()
	}
	return &ASR{
		cfg:    cfg,
		dialer: websocket.DefaultDialer,
		logger: logger.With("provider", "volc-doubao-asr"),
	}
}

func (a *ASR) Name() string { return "volc-doubao-asr" }

func (a *ASR) Ready() bool {
	return a.cfg.WSURL != "" && a.cfg.AppID != "" && a.cfg.AccessToken != "" && a.cfg.ResourceID != ""
}

func (a *ASR) Transcribe(ctx context.Context, input <-chan adapters.AudioChunk) (<-chan adapters.ASREvent, error) {
	if !a.Ready() {
		return nil, fmt.Errorf("volc asr credentials are incomplete")
	}

	connectID := uuid.NewString()
	headers := http.Header{}
	headers.Set("X-Api-App-Key", a.cfg.AppID)
	headers.Set("X-Api-Access-Key", a.cfg.AccessToken)
	headers.Set("X-Api-Resource-Id", a.cfg.ResourceID)
	headers.Set("X-Api-Connect-Id", connectID)
	headers.Set("X-Api-Request-Id", connectID)

	conn, resp, err := a.dialer.DialContext(ctx, a.cfg.WSURL, headers)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			body := strings.TrimSpace(string(data))
			if body != "" {
				return nil, fmt.Errorf("dial volc asr: %w (status=%s body=%s)", err, resp.Status, body)
			}
			return nil, fmt.Errorf("dial volc asr: %w (status=%s)", err, resp.Status)
		}
		return nil, fmt.Errorf("dial volc asr: %w", err)
	}

	a.logger.Info(
		"volc asr websocket connected",
		"url", a.cfg.WSURL,
		"connect_id", connectID,
		"resource_id", a.cfg.ResourceID,
		"sample_rate", a.cfg.SampleRate,
		"chunk_ms", a.chunkMS(),
		"tt_logid", headerValue(resp, "X-Tt-Logid"),
	)

	out := make(chan adapters.ASREvent, 16)
	go a.runSession(ctx, conn, input, out, connectID)
	return out, nil
}

func (a *ASR) chunkMS() int {
	if a.cfg.ChunkMS <= 0 {
		return 200
	}
	return a.cfg.ChunkMS
}

type volcASRResult struct {
	Text       string             `json:"text"`
	Utterances []volcASRUtterance `json:"utterances"`
}

type volcASRPayload struct {
	Result volcASRResult `json:"result"`
}

type volcASRUtterance struct {
	Text     string `json:"text"`
	Definite bool   `json:"definite"`
}

func (a *ASR) runSession(ctx context.Context, conn *websocket.Conn, input <-chan adapters.AudioChunk, out chan<- adapters.ASREvent, connectID string) {
	defer close(out)
	defer conn.Close()

	sendErr := make(chan error, 1)
	recvErr := make(chan error, 1)

	go func() { sendErr <- a.sendLoop(ctx, conn, input, connectID) }()
	go func() { recvErr <- a.recvLoop(ctx, conn, out, connectID) }()

	var once sync.Once
	closeWS := func() {
		once.Do(func() {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(250*time.Millisecond))
		})
	}

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("volc asr session cancelled", "err", ctx.Err())
			closeWS()
			return
		case err := <-sendErr:
			sendErr = nil
			if err != nil {
				a.logger.Info("volc asr send loop finished", "err", err)
			} else {
				a.logger.Info("volc asr send loop finished")
			}
			closeWS()
		case err := <-recvErr:
			if err != nil {
				a.logger.Info("volc asr recv loop finished", "err", err)
			} else {
				a.logger.Info("volc asr recv loop finished")
			}
			closeWS()
			return
		}
	}
}

func (a *ASR) sendLoop(ctx context.Context, conn *websocket.Conn, input <-chan adapters.AudioChunk, connectID string) error {
	// Full client request (sequence=1).
	fullReq := map[string]any{
		"user": map[string]any{
			"uid": uuid.NewString(),
		},
		"audio":   a.audioConfig(),
		"request": a.requestConfig(),
	}
	fullJSON, err := json.Marshal(fullReq)
	if err != nil {
		return err
	}
	fullCompressed, err := gzipBytes(fullJSON)
	if err != nil {
		return err
	}
	seq := int32(1)
	msg := make([]byte, 0, 4+4+4+len(fullCompressed))
	h := makeHeader(messageTypeFullClientRequest, flagHasSequence, serializationJSON, compressionGZIP)
	msg = append(msg, h[:]...)
	msg = appendI32BE(msg, seq)
	msg = appendU32BE(msg, uint32(len(fullCompressed)))
	msg = append(msg, fullCompressed...)
	if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		return err
	}
	a.logger.Info(
		"volc asr full client request sent",
		"connect_id", connectID,
		"sequence", seq,
		"payload_bytes", len(fullCompressed),
		"language", a.cfg.Language,
		"bidirectional", a.isBidirectionalMode(),
		"optimized", a.isOptimizedBidirectionalMode(),
	)

	// Audio packets start at sequence=2.
	seq = 2
	targetBytes := int(a.cfg.SampleRate) * 2 * a.chunkMS() / 1000
	if targetBytes <= 0 {
		targetBytes = 6400
	}

	var buf []byte
	flush := func(last bool) error {
		flags := byte(flagHasSequence)
		s := seq
		if last {
			flags = flagNegWithSequence
			s = -seq
		}

		compression := byte(compressionGZIP)
		payload := []byte(nil)
		if len(buf) > 0 {
			encoded, err := gzipBytes(buf)
			if err != nil {
				return err
			}
			payload = encoded
		} else {
			// Some implementations send an empty last packet (payload_size=0).
			// Keeping it uncompressed avoids surprising non-zero payload sizes.
			compression = compressionNone
		}
		m := make([]byte, 0, 4+4+4+len(payload))
		h := makeHeader(messageTypeAudioOnlyRequest, flags, serializationNone, compression)
		m = append(m, h[:]...)
		m = appendI32BE(m, s)
		m = appendU32BE(m, uint32(len(payload)))
		m = append(m, payload...)

		if err := conn.WriteMessage(websocket.BinaryMessage, m); err != nil {
			return err
		}
		audioMS := len(buf) * 1000 / (int(a.cfg.SampleRate) * 2)
		a.logger.Info(
			"volc asr audio packet sent",
			"connect_id", connectID,
			"sequence", s,
			"last", last,
			"audio_bytes", len(buf),
			"wire_bytes", len(payload),
			"audio_ms", audioMS,
			"compression", compression,
		)
		seq++
		buf = buf[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-input:
			if !ok {
				// Final packet. If we have residual audio, send it as last; otherwise send an empty last packet.
				return flush(true)
			}
			if len(chunk.PCM) == 0 {
				continue
			}
			buf = append(buf, chunk.PCM...)
			if len(buf) >= targetBytes {
				if err := flush(false); err != nil {
					return err
				}
			}
		}
	}
}

func (a *ASR) recvLoop(ctx context.Context, conn *websocket.Conn, out chan<- adapters.ASREvent, connectID string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		pkt, err := parsePacket(data)
		if err != nil {
			a.logger.Error("parse volc asr packet failed", "err", err)
			continue
		}
		a.logger.Info(
			"volc asr packet received",
			"connect_id", connectID,
			"message_type", pkt.MessageType,
			"flags", pkt.Flags,
			"payload_bytes", len(pkt.Payload),
			"err_code", uint32Value(pkt.ErrCode),
		)

		switch pkt.MessageType {
		case messageTypeServerACK:
			// ignore
			continue
		case messageTypeServerError:
			// payload is json describing the error.
			fields := []any{"payload", string(pkt.Payload)}
			if pkt.ErrCode != nil {
				fields = append(fields, "code", *pkt.ErrCode)
			}
			a.logger.Error("volc asr server error", fields...)
			if pkt.ErrCode != nil {
				return fmt.Errorf("volc asr server error code=%d", *pkt.ErrCode)
			}
			return fmt.Errorf("volc asr server error")
		case messageTypeFullServerResponse:
		default:
			continue
		}

		var payload volcASRPayload
		if err := json.Unmarshal(pkt.Payload, &payload); err != nil {
			a.logger.Error("decode volc asr payload failed", "err", err, "payload", string(pkt.Payload))
			continue
		}
		events := a.eventsFromPayload(pkt, payload)
		if len(events) == 0 {
			// Some responses only carry metadata.
			if pkt.Flags&flagIsLastPackage != 0 {
				a.logger.Info("volc asr final packet without transcript", "connect_id", connectID)
				out <- adapters.ASREvent{Final: true}
				return nil
			}
			continue
		}

		for _, event := range events {
			a.logger.Info(
				"volc asr transcript packet decoded",
				"connect_id", connectID,
				"final", event.Final,
				"text_len", len(event.Text),
				"text", event.Text,
			)
			out <- event
			if event.Final {
				return nil
			}
		}
	}
}

func (a *ASR) isBidirectionalMode() bool {
	if a.isNoStreamMode() {
		return false
	}
	return a.isOptimizedBidirectionalMode() || strings.Contains(a.cfg.WSURL, "/bigmodel")
}

func (a *ASR) isOptimizedBidirectionalMode() bool {
	return strings.Contains(a.cfg.WSURL, "/bigmodel_async")
}

func (a *ASR) isNoStreamMode() bool {
	return strings.Contains(a.cfg.WSURL, "/bigmodel_nostream")
}

func (a *ASR) audioConfig() map[string]any {
	audio := map[string]any{
		"format":  "pcm",
		"rate":    a.cfg.SampleRate,
		"bits":    16,
		"channel": 1,
	}
	if a.isNoStreamMode() && a.cfg.Language != "" {
		audio["language"] = a.cfg.Language
	}
	return audio
}

func (a *ASR) requestConfig() map[string]any {
	req := map[string]any{
		"model_name":  "bigmodel",
		"enable_itn":  true,
		"enable_punc": true,
		"enable_ddc":  true,
	}
	if a.isOptimizedBidirectionalMode() {
		req["show_utterances"] = true
		req["result_type"] = "single"
	}
	return req
}

func (a *ASR) eventsFromPayload(pkt parsedPacket, payload volcASRPayload) []adapters.ASREvent {
	if a.isOptimizedBidirectionalMode() && len(payload.Result.Utterances) > 0 {
		for i := len(payload.Result.Utterances) - 1; i >= 0; i-- {
			utterance := payload.Result.Utterances[i]
			text := strings.TrimSpace(utterance.Text)
			if text == "" {
				continue
			}
			return []adapters.ASREvent{{
				Text:  text,
				Final: utterance.Definite,
			}}
		}
	}

	text := strings.TrimSpace(payload.Result.Text)
	if text == "" {
		return nil
	}

	return []adapters.ASREvent{{
		Text:  text,
		Final: pkt.Flags&flagIsLastPackage != 0,
	}}
}

func headerValue(resp *http.Response, key string) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get(key)
}

func uint32Value(value *uint32) uint32 {
	if value == nil {
		return 0
	}
	return *value
}
