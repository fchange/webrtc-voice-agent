package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fchange/webrtc-voice-agent/internal/protocol/signaling"
	"github.com/gorilla/websocket"
)

const maxPendingMessages = 16

type sessionRole string

const (
	roleClient sessionRole = "client"
	roleBot    sessionRole = "bot"
)

type peer struct {
	role sessionRole
	conn *websocket.Conn
	send chan signaling.Envelope
}

type relaySession struct {
	createdAt time.Time
	peers     map[sessionRole]*peer
	pending   map[sessionRole][]signaling.Envelope
}

type hub struct {
	mu       sync.Mutex
	sessions map[string]*relaySession
}

func newHub() *hub {
	return &hub{sessions: make(map[string]*relaySession)}
}

func (h *hub) createSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.sessions[sessionID]; ok {
		return
	}

	h.sessions[sessionID] = &relaySession{
		createdAt: time.Now().UTC(),
		peers:     make(map[sessionRole]*peer),
		pending: map[sessionRole][]signaling.Envelope{
			roleClient: {},
			roleBot:    {},
		},
	}
}

func (h *hub) hasSession(sessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	_, ok := h.sessions[sessionID]
	return ok
}

func (h *hub) createdAt(sessionID string) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return time.Time{}, false
	}

	return session.createdAt, true
}

func (h *hub) attach(sessionID string, role sessionRole, conn *websocket.Conn) (*peer, []signaling.Envelope, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return nil, nil, errors.New("session not found")
	}

	if _, exists := session.peers[role]; exists {
		return nil, nil, fmt.Errorf("role %s already attached", role)
	}

	p := &peer{
		role: role,
		conn: conn,
		send: make(chan signaling.Envelope, maxPendingMessages),
	}
	session.peers[role] = p

	queued := append([]signaling.Envelope(nil), session.pending[role]...)
	session.pending[role] = nil
	return p, queued, nil
}

func (h *hub) detach(sessionID string, role sessionRole) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return
	}

	if p, exists := session.peers[role]; exists {
		delete(session.peers, role)
		close(p.send)
	}
}

func (h *hub) relay(sessionID string, from sessionRole, envelope signaling.Envelope) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return errors.New("session not found")
	}

	targetRole := oppositeRole(from)
	if target, exists := session.peers[targetRole]; exists {
		select {
		case target.send <- envelope:
			return nil
		default:
			return fmt.Errorf("target %s send queue full", targetRole)
		}
	}

	queue := append(session.pending[targetRole], envelope)
	if len(queue) > maxPendingMessages {
		queue = queue[len(queue)-maxPendingMessages:]
	}
	session.pending[targetRole] = queue
	return nil
}

func oppositeRole(role sessionRole) sessionRole {
	if role == roleClient {
		return roleBot
	}
	return roleClient
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
