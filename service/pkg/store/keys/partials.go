package keys

import (
	"fmt"
)

// Partial key format constants for prefix searches (incomplete keys)
const (
	// Version prefix for finding all versions of a message (missing ts:seq)
	VersionPrefix = "v:%s:"

	// Thread message prefix for finding all messages in a thread (missing msgID:seq)
	ThreadMessagePrefix = "t:%s:m:"

	// Thread prefix for finding all thread metadata
	ThreadPrefix = "thread:"
)

// GenAllMessageVersionsPrefix returns prefix for searching all versions of a message
func GenAllMessageVersionsPrefix(msgID string) string {
	return fmt.Sprintf(VersionPrefix, msgID)
}

// GenAllThreadMessagesPrefix returns prefix for searching all messages in a thread
func GenAllThreadMessagesPrefix(threadID string) string {
	return fmt.Sprintf(ThreadMessagePrefix, threadID)
}

// GenThreadPrefix returns prefix for searching all thread metadata
func GenThreadPrefix() string {
	return ThreadPrefix
}
