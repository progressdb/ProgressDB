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

	// Just user relationships
	UserThreadsRelPrefix = "rel:u:"

	// User -> thread relationship prefix for all threads for a user (formerly in function GenUserThreadRelPrefix)
	UserThreadRelPrefix = "rel:u:%s:t:"

	// Thread -> user relationship prefix for all users for a thread (formerly in function GenThreadUserRelPrefix)
	ThreadUserRelPrefix = "rel:t:%s:u:"

	// Backup before encryption
	BackupEncryptPrefix = "backup:encrypt:"

	// WALMetaNextSequenceKey is the key for storing next sequence in WAL metadata
	WALMetaNextSequenceKey = "meta:next_sequence"

	// TempIndexKeyFormat is the format for temporary index keys during recovery
	TempIndexKeyFormat = "temp_idx:%s:%s"

	// RecoveryIndexKeyFormat is the format for final index keys during recovery
	RecoveryIndexKeyFormat = "idx:%s:%s"

	// TempIndexPrefix is the prefix for temporary index keys
	TempIndexPrefix = "temp_idx:"

	// TempIndexUpperBound is the upper bound for temporary index key scans
	TempIndexUpperBound = "temp_idx;"
)

func GenAllMessageVersionsPrefix(msgID string) string {
	return fmt.Sprintf(VersionPrefix, msgID)
}

func GenAllThreadMessagesPrefix(threadID string) string {
	return fmt.Sprintf(ThreadMessagePrefix, threadID)
}

func GenThreadMetadataPrefix() string {
	return ThreadMetadataPrefix
}

func GenThreadMessagesGEPrefix(threadID string, seq uint64) string {
	return fmt.Sprintf(ThreadMessageGEPrefix, threadID, PadSeq(seq))
}

func GenUserThreadRelPrefix(userID string) string {
	return fmt.Sprintf(UserThreadRelPrefix, userID)
}

func GenThreadUserRelPrefix(threadID string) string {
	return fmt.Sprintf(ThreadUserRelPrefix, threadID)
}

func ParseVersionKeySequence(key string) (uint64, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return 0, err
	}
	if parsed.Type != KeyTypeVersion {
		return 0, fmt.Errorf("expected version key, got %s", parsed.Type)
	}
	return ParseKeySequence(parsed.Seq)
}
