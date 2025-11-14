package tracking

import (
	"sync"
)

var GlobalInflightTracker *InflightTracker

type InflightTracker struct {
	keys map[string]chan struct{} // provisionalKey -> close channel when done
	mu   sync.RWMutex
}

func NewInflightTracker() *InflightTracker {
	return &InflightTracker{
		keys: make(map[string]chan struct{}),
	}
}

func InitGlobalInflightTracker() {
	GlobalInflightTracker = NewInflightTracker()
}

// Add starts tracking a provisional key
func (t *InflightTracker) Add(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.keys[key] = make(chan struct{})
}

// Remove stops tracking a provisional key and wakes up all waiters
func (t *InflightTracker) Remove(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if ch, exists := t.keys[key]; exists {
		delete(t.keys, key)
		close(ch) // Wake up all waiters
	}
}

// IsInflight checks if a key is currently being tracked
func (t *InflightTracker) IsInflight(key string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.keys[key]
	return exists
}

// WaitForInflight blocks until the key is no longer in-flight
func (t *InflightTracker) WaitForInflight(key string) {
	t.mu.RLock()
	ch, exists := t.keys[key]
	t.mu.RUnlock()
	
	if exists {
		<-ch // Block until channel is closed
	}
}
