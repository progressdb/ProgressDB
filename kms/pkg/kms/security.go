package kms

import (
	"sync"
)

// secureBytes provides secure memory handling for sensitive data
type secureBytes struct {
	data []byte
	mu   sync.RWMutex
}

func newSecureBytes(data []byte) *secureBytes {
	// Make a copy to avoid external references
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return &secureBytes{data: dataCopy}
}

func (s *secureBytes) Data() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to avoid external modifications
	dataCopy := make([]byte, len(s.data))
	copy(dataCopy, s.data)
	return dataCopy
}

func (s *secureBytes) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Securely wipe the data
	if s.data != nil {
		for i := range s.data {
			s.data[i] = 0
		}
		s.data = nil
	}
}

// secureWipe securely wipes sensitive data from memory
func secureWipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
