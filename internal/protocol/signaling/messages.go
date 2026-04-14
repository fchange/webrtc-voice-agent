package signaling

import (
	"encoding/json"
	"errors"

	protoerrors "github.com/fchange/webrtc-voice-agent/internal/protocol/errors"
)

const Version = "v1alpha1"

type MessageType string

const (
	TypeSessionCreate   MessageType = "session.create"
	TypeSessionCreated  MessageType = "session.created"
	TypeSessionAttach   MessageType = "session.attach"
	TypeSessionOffer    MessageType = "session.offer"
	TypeSessionAnswer   MessageType = "session.answer"
	TypeSessionICE      MessageType = "session.ice_candidate"
	TypeSessionClose    MessageType = "session.close"
	TypeSessionError    MessageType = "session.error"
	TypeSessionAttached MessageType = "session.attached"
)

type Envelope struct {
	Version   string          `json:"version"`
	Type      MessageType     `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type CreateSessionRequest struct {
	ClientID string `json:"client_id,omitempty"`
}

type CreateSessionResponse struct {
	SessionID      string   `json:"session_id"`
	SignalingWSURL string   `json:"signaling_ws_url"`
	ICEUrls        []string `json:"ice_urls"`
	TokenHint      string   `json:"token_hint,omitempty"`
}

type AttachPayload struct {
	Role string `json:"role"`
}

type SDPPayload struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

type ICECandidatePayload struct {
	Candidate     string  `json:"candidate"`
	SDPMid        string  `json:"sdp_mid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdp_mline_index,omitempty"`
}

type ErrorPayload struct {
	Code    protoerrors.Code `json:"code"`
	Message string           `json:"message"`
}

func (e Envelope) Validate() error {
	if e.Version == "" {
		return errors.New("missing version")
	}
	if e.Version != Version {
		return errors.New("unsupported version")
	}
	if e.Type == "" {
		return errors.New("missing type")
	}
	return nil
}
