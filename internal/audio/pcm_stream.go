package audio

import "sync"

type PCMFrameStream struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan PCMFrame
	closed      bool
}

func NewPCMFrameStream() *PCMFrameStream {
	return &PCMFrameStream{
		subscribers: make(map[int]chan PCMFrame),
	}
}

func (s *PCMFrameStream) Subscribe(buffer int) (<-chan PCMFrame, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan PCMFrame, buffer)
	if s.closed {
		close(ch)
		return ch, func() {}
	}

	id := s.nextID
	s.nextID++
	s.subscribers[id] = ch

	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		current, ok := s.subscribers[id]
		if !ok {
			return
		}
		delete(s.subscribers, id)
		close(current)
	}
}

func (s *PCMFrameStream) Publish(frame PCMFrame) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return
	}

	for _, subscriber := range s.subscribers {
		select {
		case subscriber <- frame:
		default:
		}
	}
}

func (s *PCMFrameStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	for id, subscriber := range s.subscribers {
		close(subscriber)
		delete(s.subscribers, id)
	}
}
