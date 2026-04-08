package audio

import "sync"

type EncodedPacketStream struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan EncodedPacket
	closed      bool
}

func NewEncodedPacketStream() *EncodedPacketStream {
	return &EncodedPacketStream{
		subscribers: make(map[int]chan EncodedPacket),
	}
}

func (s *EncodedPacketStream) Subscribe(buffer int) (<-chan EncodedPacket, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan EncodedPacket, buffer)
	if s.closed {
		close(ch)
		return ch, func() {}
	}

	id := s.nextID
	s.nextID++
	s.subscribers[id] = ch

	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		if subscriber, ok := s.subscribers[id]; ok {
			delete(s.subscribers, id)
			close(subscriber)
		}
	}

	return ch, cancel
}

func (s *EncodedPacketStream) Publish(packet EncodedPacket) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return
	}

	for _, subscriber := range s.subscribers {
		select {
		case subscriber <- packet:
		default:
		}
	}
}

func (s *EncodedPacketStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	for id, subscriber := range s.subscribers {
		delete(s.subscribers, id)
		close(subscriber)
	}
}
