package store

import (
	"bytes"
	"sync"

	"github.com/cockroachdb/pebble"
)

var pendingWrites uint64
var seq uint64

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

// scans messages for thread and returns largest sequence number
func computeMaxSeq(threadID string) (uint64, error) {
	mp, merr := MsgPrefix(threadID)
	if merr != nil {
		return 0, merr
	}
	prefix := []byte(mp)
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	var max uint64
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := string(iter.Key())
		_, _, s, perr := ParseMsgKey(k)
		if perr != nil {
			continue
		}
		if s > max {
			max = s
		}
	}
	return max, iter.Error()
}

// wrapper for computeMaxSeq (for migrations/admin)
func MaxSeqForThread(threadID string) (uint64, error) {
	return computeMaxSeq(threadID)
}
