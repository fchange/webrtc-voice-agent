package session

import (
	"testing"
	"time"
)

func TestManagerCloseIdle(t *testing.T) {
	manager := NewManager(time.Millisecond)
	task := manager.Create("sess_1")
	if task == nil {
		t.Fatal("expected task")
	}

	time.Sleep(3 * time.Millisecond)
	closed := manager.CloseIdle(time.Now().UTC())
	if len(closed) != 1 || closed[0] != "sess_1" {
		t.Fatalf("expected sess_1 to be closed, got %v", closed)
	}
}
