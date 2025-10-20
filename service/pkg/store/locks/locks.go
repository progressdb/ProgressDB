package store

import (
	"sync"
)

var (
	threadLocks = make(map[string]*sync.Mutex)
	locksMu     sync.Mutex
)

// returns mutex for given thread (creates if needed)
func getThreadLock(threadID string) *sync.Mutex {
	locksMu.Lock()
	defer locksMu.Unlock()
	if l, ok := threadLocks[threadID]; ok {
		return l
	}
	l := &sync.Mutex{}
	threadLocks[threadID] = l
	return l
}
