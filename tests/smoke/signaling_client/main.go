package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/protocol/signaling"
)

func main() {
	httpURL := flag.String("http-url", "http://127.0.0.1:18080", "signal http base url")
	wsURL := flag.String("ws-url", "ws://127.0.0.1:18080/ws", "signal websocket url")
	token := flag.String("token", "dev-token", "signal dev token")
	flag.Parse()

	sessionID, err := createSession(*httpURL, *token)
	if err != nil {
		log.Fatal(err)
	}

	queryURL := fmt.Sprintf("%s?session_id=%s&role=client&access_token=%s", *wsURL, sessionID, *token)
	conn, _, err := websocket.DefaultDialer.Dial(queryURL, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		log.Fatal(err)
	}

	var envelope signaling.Envelope
	if err := conn.ReadJSON(&envelope); err != nil {
		log.Fatal(err)
	}
	if envelope.Type != signaling.TypeSessionAttached {
		log.Fatalf("expected %s, got %s", signaling.TypeSessionAttached, envelope.Type)
	}
	log.Printf("received %s for session %s", envelope.Type, sessionID)

	closeEnvelope := signaling.Envelope{
		Version:   signaling.Version,
		Type:      signaling.TypeSessionClose,
		SessionID: sessionID,
		Payload:   mustMarshal(map[string]string{"reason": "smoke_test"}),
	}
	if err := conn.WriteJSON(closeEnvelope); err != nil {
		log.Fatal(err)
	}
	log.Printf("sent %s for session %s", closeEnvelope.Type, sessionID)

	for {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			log.Fatal(err)
		}

		if err := conn.ReadJSON(&envelope); err != nil {
			var netErr interface{ Timeout() bool }
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Printf("no further websocket messages within timeout; smoke check complete")
				return
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("websocket closed after session.close")
				return
			}
			log.Fatal(err)
		}

		select {
		case <-ctx.Done():
			log.Printf("smoke check complete")
			return
		default:
			log.Printf("received websocket message type=%s session_id=%s", envelope.Type, envelope.SessionID)
		}
	}
}

func createSession(baseURL string, token string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/sessions", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create session failed: status=%d", resp.StatusCode)
	}

	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.SessionID == "" {
		return "", fmt.Errorf("create session returned empty session_id")
	}
	return body.SessionID, nil
}

func mustMarshal(payload any) json.RawMessage {
	if payload == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}
