package session

import (
	"sync"
	"time"
)

type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Task
	idleTimeout time.Duration
}

func NewManager(idleTimeout time.Duration) *Manager {
	return &Manager{
		sessions:    make(map[string]*Task),
		idleTimeout: idleTimeout,
	}
}

func (m *Manager) Create(sessionID string) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := NewTask(sessionID, m.idleTimeout)
	m.sessions[sessionID] = task
	return task
}

func (m *Manager) Get(sessionID string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.sessions[sessionID]
	return task, ok
}

func (m *Manager) End(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	_ = task.End()
	delete(m.sessions, sessionID)
	return true
}

func (m *Manager) CloseIdle(now time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	closed := make([]string, 0)
	for sessionID, task := range m.sessions {
		if task.Expired(now) {
			_ = task.End()
			delete(m.sessions, sessionID)
			closed = append(closed, sessionID)
		}
	}

	return closed
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.sessions)
}
