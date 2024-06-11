package internal

import "sync"

type AtomicString struct {
	mux sync.RWMutex
	val string
}

func (s *AtomicString) Load() string {
	s.mux.RLock()
	defer s.mux.RUnlock()
	return s.val
}

func (s *AtomicString) Store(newVal string) {
	s.mux.Lock()
	s.val = newVal
	s.mux.Unlock()
}
