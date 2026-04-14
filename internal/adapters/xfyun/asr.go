package xfyun

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	"github.com/fchange/webrtc-voice-agent/internal/config"
	"github.com/gorilla/websocket"
)

type ASR struct {
	cfg    config.XFYUNASRConfig
	dialer *websocket.Dialer
	clock  func() time.Time
	logger *slog.Logger
}

func NewASR(cfg config.XFYUNASRConfig, logger *slog.Logger) *ASR {
	if logger == nil {
		logger = slog.Default()
	}
	return &ASR{
		cfg:    cfg,
		dialer: websocket.DefaultDialer,
		clock:  time.Now,
		logger: logger.With("provider", "xfyun-spark-iat"),
	}
}

func (a *ASR) Name() string {
	return "xfyun-spark-iat"
}

func (a *ASR) Ready() bool {
	return a.cfg.AppID != "" && a.cfg.APIKey != "" && a.cfg.APISecret != ""
}

func (a *ASR) Transcribe(ctx context.Context, input <-chan adapters.AudioChunk) (<-chan adapters.ASREvent, error) {
	if !a.Ready() {
		return nil, fmt.Errorf("xfyun asr credentials are incomplete")
	}

	authURL, err := BuildAuthURL(a.cfg.WSURL, a.cfg.Host, a.cfg.RequestPath, a.cfg.APIKey, a.cfg.APISecret, a.clock().UTC())
	if err != nil {
		return nil, err
	}

	conn, resp, err := a.dialer.DialContext(ctx, authURL, http.Header{})
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial xfyun asr: %w (status=%s)", err, resp.Status)
		}
		return nil, fmt.Errorf("dial xfyun asr: %w", err)
	}

	a.logger.Info(
		"xfyun asr websocket connected",
		"url", a.cfg.WSURL,
		"domain", a.cfg.Domain,
		"language", a.cfg.Language,
		"sample_rate", a.cfg.SampleRate,
		"encoding", a.cfg.AudioEncoding,
	)

	out := make(chan adapters.ASREvent, 16)
	go a.runSession(ctx, conn, input, out)
	return out, nil
}

func (a *ASR) runSession(ctx context.Context, conn *websocket.Conn, input <-chan adapters.AudioChunk, out chan<- adapters.ASREvent) {
	defer close(out)
	defer conn.Close()

	const finalResponseTimeout = 5 * time.Second

	var once sync.Once
	closeWithReason := func() {
		once.Do(func() {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(250*time.Millisecond))
		})
	}

	accumulator := newTranscriptAccumulator()
	sendErr := make(chan error, 1)
	recvErr := make(chan error, 1)

	go func() {
		recvErr <- a.recvLoop(ctx, conn, accumulator, out)
	}()

	go func() {
		sendErr <- a.sendLoop(ctx, conn, input)
	}()

	var finalWait *time.Timer
	stopFinalWait := func() {
		if finalWait == nil {
			return
		}
		if !finalWait.Stop() {
			select {
			case <-finalWait.C:
			default:
			}
		}
	}
	defer stopFinalWait()

	for {
		var finalWaitC <-chan time.Time
		if finalWait != nil {
			finalWaitC = finalWait.C
		}

		select {
		case <-ctx.Done():
			a.logger.Info("xfyun asr session cancelled", "err", ctx.Err())
			closeWithReason()
			return
		case err := <-sendErr:
			sendErr = nil
			if err != nil {
				a.logger.Info("xfyun asr send loop finished", "err", err)
				closeWithReason()
				return
			}

			a.logger.Info("xfyun asr send loop finished", "waiting_for_final_response", true)
			stopFinalWait()
			finalWait = time.NewTimer(finalResponseTimeout)
		case err := <-recvErr:
			if err != nil {
				a.logger.Info("xfyun asr recv loop finished", "err", err)
			} else {
				a.logger.Info("xfyun asr recv loop finished")
			}
			closeWithReason()
			return
		case <-finalWaitC:
			a.logger.Warn("xfyun asr final response timeout", "timeout_ms", finalResponseTimeout.Milliseconds())
			closeWithReason()
			return
		}
	}
}

func (a *ASR) sendLoop(ctx context.Context, conn *websocket.Conn, input <-chan adapters.AudioChunk) error {
	seq := 1
	first := true
	chunkCount := 0
	byteCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-input:
			if !ok {
				a.logger.Info("xfyun asr sending final frame", "chunks", chunkCount, "bytes", byteCount, "seq", seq)
				return conn.WriteJSON(newAudioRequest(a.cfg, seq, 2, nil, first))
			}

			status := 1
			if first {
				status = 0
			}
			if first {
				a.logger.Info("xfyun asr sending first frame", "seq", seq, "bytes", len(chunk.PCM), "timestamp_ms", chunk.Timestamp.Milliseconds())
			}
			if err := conn.WriteJSON(newAudioRequest(a.cfg, seq, status, chunk.PCM, first)); err != nil {
				return err
			}
			first = false
			seq++
			chunkCount++
			byteCount += len(chunk.PCM)
		}
	}
}

func (a *ASR) recvLoop(ctx context.Context, conn *websocket.Conn, accumulator *transcriptAccumulator, out chan<- adapters.ASREvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var response responseEnvelope
		if err := conn.ReadJSON(&response); err != nil {
			return err
		}
		a.logger.Info(
			"xfyun asr response received",
			"sid", response.Header.Sid,
			"code", response.Header.Code,
			"status", response.Header.Status,
			"result_status", response.Payload.Result.Status,
			"result_seq", response.Payload.Result.Seq,
			"has_text", response.Payload.Result.Text != "",
		)
		if response.Header.Code != 0 {
			return fmt.Errorf("xfyun code=%d message=%s", response.Header.Code, response.Header.Message)
		}
		if response.Payload.Result.Text == "" {
			if response.Header.Status == 2 {
				a.logger.Info("xfyun asr final response arrived without transcript text", "sid", response.Header.Sid)
				return nil
			}
			continue
		}

		decoded, err := decodeResultText(response.Payload.Result.Text)
		if err != nil {
			return err
		}
		text := accumulator.Apply(decoded, response.Payload.Result)
		a.logger.Info(
			"xfyun asr transcript decoded",
			"sid", response.Header.Sid,
			"sn", decoded.SN,
			"ls", decoded.LS,
			"rst", decoded.RST,
			"pgs", response.Payload.Result.PGS,
			"rg", response.Payload.Result.RG,
			"result_seq", response.Payload.Result.Seq,
			"final", decoded.LS || response.Header.Status == 2 || response.Payload.Result.Status == 2,
			"text_len", len(text),
			"text", text,
		)
		out <- adapters.ASREvent{
			Text:  text,
			Final: decoded.LS || response.Header.Status == 2 || response.Payload.Result.Status == 2,
		}
		if decoded.LS || response.Header.Status == 2 || response.Payload.Result.Status == 2 {
			return nil
		}
	}
}

func newAudioRequest(cfg config.XFYUNASRConfig, seq int, status int, pcm []byte, first bool) requestEnvelope {
	request := requestEnvelope{
		Header: requestHeader{
			AppID:  cfg.AppID,
			Status: status,
		},
		Payload: requestPayload{
			Audio: requestAudio{
				Encoding:   cfg.AudioEncoding,
				SampleRate: cfg.SampleRate,
				Channels:   1,
				BitDepth:   16,
				Seq:        seq,
				Status:     status,
				Audio:      base64.StdEncoding.EncodeToString(pcm),
			},
		},
	}

	if first {
		request.Parameter = &requestParameter{
			IAT: requestIAT{
				Domain:   cfg.Domain,
				Language: cfg.Language,
				Accent:   cfg.Accent,
				EOSMS:    cfg.EOSMS,
				DWA:      cfg.DWA,
				Result: requestResult{
					Encoding: "utf8",
					Compress: "raw",
					Format:   "json",
				},
			},
		}
	}

	return request
}

func decodeResultText(encoded string) (decodedResult, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return decodedResult{}, err
	}

	var result decodedResult
	if err := json.Unmarshal(data, &result); err != nil {
		return decodedResult{}, err
	}
	return result, nil
}
