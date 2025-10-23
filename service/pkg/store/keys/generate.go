package keys

import (
	"fmt"
	"strconv"
	"strings"
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

// Generate a provisional message id in the form "msg-<timestamp>", using the provided timestamp.
func GenProvisionalMessageID(ts int64) string {
	return fmt.Sprintf("msg-%d", ts)
}

// Generate a final thread ID with user-specific sequence in the form "thread-<timestamp>-<userSeq>"
func GenUserThreadID(ts int64, userSeq uint64) string {
	return fmt.Sprintf("thread-%d-%d", ts, userSeq)
}

// IsFinalThreadID checks if a thread ID is a final ID (has sequence component)
func IsFinalThreadID(threadID string) bool {
	// Final IDs have format: thread-<timestamp>-<sequence>
	// Provisional IDs have format: thread-<timestamp>
	parts := strings.Split(threadID, "-")
	return len(parts) == 3 && parts[0] == "thread"
}

// IsFinalMessageID checks if a message ID is a final ID (has sequence component)
func IsFinalMessageID(messageID string) bool {
	// Final IDs have format: msg-<timestamp>-<sequence>
	// Provisional IDs would have format: msg-<timestamp>
	parts := strings.Split(messageID, "-")
	return len(parts) == 3 && parts[0] == "msg"
}

// ExtractTimestampFromProvisionalID extracts timestamp from provisional thread ID
func ExtractTimestampFromProvisionalID(provisionalID string) (int64, error) {
	// Expected format: thread-<timestamp>
	parts := strings.Split(provisionalID, "-")
	if len(parts) != 2 || parts[0] != "thread" {
		return 0, fmt.Errorf("invalid provisional thread ID format: %s", provisionalID)
	}

	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp in provisional ID: %w", err)
	}

	return timestamp, nil
}
