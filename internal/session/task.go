package session

import (
	"fmt"
	"sync"
	"time"
)

type State string

const (
	StateCreated     State = "created"
	StateNegotiating State = "negotiating"
	StateActive      State = "active"
	StateProcessing  State = "processing"
	StateResponding  State = "responding"
	StateClosing     State = "closing"
	StateClosed      State = "closed"
)

type InterruptResult struct {
	InterruptedTurnID int64  `json:"interrupted_turn_id"`
	NextTurnID        int64  `json:"next_turn_id"`
	Reason            string `json:"reason"`
}

type Snapshot struct {
	SessionID   string    `json:"session_id"`
	State       State     `json:"state"`
	CurrentTurn int64     `json:"current_turn"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Task struct {
	mu          sync.Mutex
	sessionID   string
	state       State
	currentTurn int64
	updatedAt   time.Time
	idleTimeout time.Duration
}

func NewTask(sessionID string, idleTimeout time.Duration) *Task {
	return &Task{
		sessionID:   sessionID,
		state:       StateCreated,
		updatedAt:   time.Now().UTC(),
		idleTimeout: idleTimeout,
	}
}

func (t *Task) BeginNegotiation() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateCreated {
		return fmt.Errorf("cannot negotiate from %s", t.state)
	}

	t.state = StateNegotiating
	t.touch()
	return nil
}

func (t *Task) MarkActive() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch t.state {
	case StateNegotiating, StateProcessing, StateResponding, StateCreated:
		t.state = StateActive
		t.touch()
		return nil
	default:
		return fmt.Errorf("cannot activate from %s", t.state)
	}
}

func (t *Task) StartTurn() (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateActive {
		return 0, fmt.Errorf("cannot start turn from %s", t.state)
	}

	t.currentTurn++
	t.state = StateProcessing
	t.touch()
	return t.currentTurn, nil
}

func (t *Task) EnsureTurn() (int64, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch t.state {
	case StateActive:
		t.currentTurn++
		t.state = StateProcessing
		t.touch()
		return t.currentTurn, true, nil
	case StateProcessing, StateResponding:
		return t.currentTurn, false, nil
	default:
		return 0, false, fmt.Errorf("cannot ensure turn from %s", t.state)
	}
}

func (t *Task) StartResponse(turnID int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateProcessing {
		return fmt.Errorf("cannot respond from %s", t.state)
	}
	if turnID != t.currentTurn {
		return fmt.Errorf("turn mismatch: got %d want %d", turnID, t.currentTurn)
	}

	t.state = StateResponding
	t.touch()
	return nil
}

func (t *Task) CompleteTurn(turnID int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if turnID != t.currentTurn {
		return fmt.Errorf("turn mismatch: got %d want %d", turnID, t.currentTurn)
	}
	if t.state != StateProcessing && t.state != StateResponding {
		return fmt.Errorf("cannot complete from %s", t.state)
	}

	t.state = StateActive
	t.touch()
	return nil
}

func (t *Task) Interrupt(reason string) (InterruptResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateProcessing && t.state != StateResponding {
		return InterruptResult{}, fmt.Errorf("cannot interrupt from %s", t.state)
	}

	current := t.currentTurn
	t.state = StateActive
	t.touch()

	return InterruptResult{
		InterruptedTurnID: current,
		NextTurnID:        current + 1,
		Reason:            reason,
	}, nil
}

func (t *Task) End() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state == StateClosed {
		return nil
	}

	t.state = StateClosing
	t.touch()
	t.state = StateClosed
	t.touch()
	return nil
}

func (t *Task) Expired(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state == StateClosed {
		return false
	}

	return now.Sub(t.updatedAt) > t.idleTimeout
}

func (t *Task) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	return Snapshot{
		SessionID:   t.sessionID,
		State:       t.state,
		CurrentTurn: t.currentTurn,
		UpdatedAt:   t.updatedAt,
	}
}

func (t *Task) touch() {
	t.updatedAt = time.Now().UTC()
}
