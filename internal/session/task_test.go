package session

import (
	"testing"
	"time"
)

func TestTaskInterruptAdvancesTurn(t *testing.T) {
	task := NewTask("sess_1", time.Minute)

	if err := task.BeginNegotiation(); err != nil {
		t.Fatalf("begin negotiation: %v", err)
	}
	if err := task.MarkActive(); err != nil {
		t.Fatalf("mark active: %v", err)
	}

	turnID, err := task.StartTurn()
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	if err := task.StartResponse(turnID); err != nil {
		t.Fatalf("start response: %v", err)
	}

	result, err := task.Interrupt("user_barge_in")
	if err != nil {
		t.Fatalf("interrupt: %v", err)
	}

	if result.InterruptedTurnID != 1 {
		t.Fatalf("expected interrupted turn 1, got %d", result.InterruptedTurnID)
	}
	if result.NextTurnID != 2 {
		t.Fatalf("expected next turn 2, got %d", result.NextTurnID)
	}

	snapshot := task.Snapshot()
	if snapshot.State != StateActive {
		t.Fatalf("expected active state after interrupt, got %s", snapshot.State)
	}
}

func TestTaskEndClosesSession(t *testing.T) {
	task := NewTask("sess_1", time.Minute)
	if err := task.End(); err != nil {
		t.Fatalf("end: %v", err)
	}

	if got := task.Snapshot().State; got != StateClosed {
		t.Fatalf("expected closed state, got %s", got)
	}
}
