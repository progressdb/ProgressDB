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

	UserThreadsRelPrefix = "rel:u:"
	BackupEncryptPrefix  = "backup:encrypt:"
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
	return fmt.Sprintf("rel:u:%s:t:", userID)
}

func GenThreadUserRelPrefix(threadID string) string {
	return fmt.Sprintf("rel:t:%s:u:", threadID)
}

func ParseVersionKeySequence(key string) (uint64, error) {
	parts, err := ParseVersionKey(key)
	if err != nil {
		return 0, err
	}
	return ParseKeySequence(parts.Seq)
}
