package xfyun

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

type ASR struct {
	cfg    config.XFYUNASRConfig
	dialer *websocket.Dialer
	clock  func() time.Time
}

func NewASR(cfg config.XFYUNASRConfig) *ASR {
	return &ASR{
		cfg:    cfg,
		dialer: websocket.DefaultDialer,
		clock:  time.Now,
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

	out := make(chan adapters.ASREvent, 16)
	go a.runSession(ctx, conn, input, out)
	return out, nil
}

func (a *ASR) runSession(ctx context.Context, conn *websocket.Conn, input <-chan adapters.AudioChunk, out chan<- adapters.ASREvent) {
	defer close(out)
	defer conn.Close()

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

	select {
	case <-ctx.Done():
		closeWithReason()
	case <-sendErr:
		closeWithReason()
	case <-recvErr:
		closeWithReason()
	}
}

func (a *ASR) sendLoop(ctx context.Context, conn *websocket.Conn, input <-chan adapters.AudioChunk) error {
	seq := 1
	first := true

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-input:
			if !ok {
				return conn.WriteJSON(newAudioRequest(a.cfg, seq, 2, nil, first))
			}

			status := 1
			if first {
				status = 0
			}
			if err := conn.WriteJSON(newAudioRequest(a.cfg, seq, status, chunk.PCM, first)); err != nil {
				return err
			}
			first = false
			seq++
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
		if response.Header.Code != 0 {
			return fmt.Errorf("xfyun code=%d message=%s", response.Header.Code, response.Header.Message)
		}
		if response.Payload.Result.Text == "" {
			if response.Header.Status == 2 {
				return nil
			}
			continue
		}

		decoded, err := decodeResultText(response.Payload.Result.Text)
		if err != nil {
			return err
		}
		text := accumulator.Apply(decoded, response.Payload.Result)
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
