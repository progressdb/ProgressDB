package keys

import (
	"fmt"
	"sync/atomic"

	"progressdb/pkg/timeutil"
)

// Global sequence counter for ID generation.
// Down in the pipeline - this is sub scoped later with index datas
var idSeq uint64

func GetNextSequence(recordType string) uint64 {
	return atomic.AddUint64(&idSeq, 1)
}

// Returns a unique message ID in the form "msg-<timestamp>-<seq>".
func GenMessageID() string {
	n := timeutil.Now().UnixNano()
	s := GetNextSequence("message")
	return fmt.Sprintf("msg-%d-%d", n, s)
}

// Returns a unique thread ID in the form "thread-<timestamp>-<seq>".
func GenThreadID() string {
	n := timeutil.Now().UnixNano()
	s := GetNextSequence("thread")
	return fmt.Sprintf("thread-%d-%d", n, s)
}

// Generate a provisional thread id in the form "thread-<timestamp>", using the provided timestamp.
func GenProvisionalThreadID(ts int64) string {
	return fmt.Sprintf("thread-%d", ts)
}

