package keys

import (
	"fmt"
	"sync/atomic"

	"progressdb/pkg/timeutil"
)

var (
	// global sequence counter for ID generation
	idSeq uint64
)

// GetNextSequence returns the next sequence number for the given record type.
// Currently uses a global counter, but designed for future per-user or per-type sequencing.
func GetNextSequence(recordType string) uint64 {
	return atomic.AddUint64(&idSeq, 1)
}

// sequencer returns the next sequence number for a given ID.
// This enables per-ID sequencing for user-scoped key generation.
func sequencer(id string) uint64 {
	// For now, use global sequencing, but this can be extended to per-ID counters
	return GetNextSequence("sequencer")
}

// GenMessageID generates a unique message ID using the current UTC nanosecond timestamp and a sequence number.
// The format is "msg-<timestamp>-<seq>".
func GenMessageID() string {
	n := timeutil.Now().UnixNano()
	s := sequencer("message")
	return fmt.Sprintf("msg-%d-%d", n, s)
}

// GenThreadID generates a unique thread ID using the current UTC nanosecond timestamp and a sequence number.
// The format is "thread-<timestamp>-<seq>".
func GenThreadID() string {
	n := timeutil.Now().UnixNano()
	s := sequencer("thread")
	return fmt.Sprintf("thread-%d-%d", n, s)
}
