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

	// Thread metadata prefix for finding all thread metadata keys
	ThreadMetadataPrefix = "t:"

	// Thread message GE prefix for SeekGE operations (missing msgID, has seq)
	ThreadMessageGEPrefix = "t:%s:m:%s"
)

// GenAllMessageVersionsPrefix returns prefix for searching all versions of a message
func GenAllMessageVersionsPrefix(msgID string) string {
	return fmt.Sprintf(VersionPrefix, msgID)
}

// GenAllThreadMessagesPrefix returns prefix for searching all messages in a thread
func GenAllThreadMessagesPrefix(threadID string) string {
	return fmt.Sprintf(ThreadMessagePrefix, threadID)
}

// GenThreadMetadataPrefix returns prefix for searching all thread metadata keys
func GenThreadMetadataPrefix() string {
	return ThreadMetadataPrefix
}

// GenThreadMessagesGEPrefix returns prefix for SeekGE operations to start from a specific sequence
func GenThreadMessagesGEPrefix(threadID string, seq uint64) string {
	return fmt.Sprintf(ThreadMessageGEPrefix, threadID, PadSeq(seq))
}

// Relationship prefixes
const (
	UserThreadsRelPrefix = "rel:u:"
	// Backup prefix
	BackupEncryptPrefix = "backup:encrypt:"
)

// GenUserThreadRelPrefix returns prefix for listing threads owned by a user
func GenUserThreadRelPrefix(userID string) string {
	return fmt.Sprintf("rel:u:%s:t:", userID)
}

// ParseVersionKeySequence parses just the sequence from a version key using existing ParseVersionKey
func ParseVersionKeySequence(key string) (uint64, error) {
	parts, err := ParseVersionKey(key)
	if err != nil {
		return 0, err
	}
	return ParseKeySequence(parts.Seq)
}
