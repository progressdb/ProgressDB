package keys

import (
	"fmt"
)

const (
	// Used to generate a prefix for all versions of a specific message.
	VersionPrefix = "v:%s:"

	// Used to generate a prefix for all messages in a specific thread.
	ThreadMessagePrefix = "t:%s:m:"

	// Used as a simple prefix for thread metadata (e.g., for scanning all threads).
	ThreadMetadataPrefix = "t:"

	// Used for generating a lower-bound prefix for messages in a thread with a greater-than-or-equal sequence (t:{thread}:m:{seq}).
	ThreadMessageGEPrefix = "t:%s:m:%s"

	// Used for scanning all relationships between users and threads.
	UserThreadsRelPrefix = "rel:u:"

	// Used as a prefix for looking up which threads a user owns (rel:u:{userID}:t:).
	UserThreadRelPrefix = "rel:u:%s:t:"

	// Used as a prefix for looking up users in a thread (rel:t:{thread}:u:).
	ThreadUserRelPrefix = "rel:t:%s:u:"

	// Prefix used when storing keys related to backup encryption.
	BackupEncryptPrefix = "backup:encrypt:"

	// Key for storing the next sequence value for the Write-Ahead Log metadata.
	WALMetaNextSequenceKey = "meta:next_sequence"

	// Used for temporary index keys in migrations or tests (temp_idx:{table}:{id}).
	TempIndexKeyFormat = "temp_idx:%s:%s"

	// Generic format for specifying recovery index keys (idx:{type}:{id}).
	RecoveryIndexKeyFormat = "idx:%s:%s"

	// Used for scanning all temporary index keys.
	TempIndexPrefix = "temp_idx:"

	// Upper bound (exclusive limit) for iterating temporary index keys; normally "temp_idx;" due to ASCII ordering.
	TempIndexUpperBound = "temp_idx;"

	// Used for scanning all soft delete markers.
	SoftDeletePrefix = "del:"
)

func GenAllMessageVersionsPrefix(messageKey string) (string, error) {
	parsed, err := ParseKey(messageKey)
	if err != nil {
		return "", fmt.Errorf("invalid message key: %w", err)
	}
	if parsed.Type != KeyTypeMessage && parsed.Type != KeyTypeMessageProvisional {
		return "", fmt.Errorf("expected message key, got %s", parsed.Type)
	}
	return fmt.Sprintf(VersionPrefix, messageKey), nil
}

func GenAllThreadMessagesPrefix(threadKey string) (string, error) {
	parsed, err := ParseKey(threadKey)
	if err != nil {
		return "", fmt.Errorf("invalid thread key: %w", err)
	}
	if parsed.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", parsed.Type)
	}
	return fmt.Sprintf(ThreadMessagePrefix, parsed.ThreadTS), nil
}

func GenThreadMetadataPrefix() string {
	return ThreadMetadataPrefix
}

func GenThreadMessagesGEPrefix(threadKey string, seq uint64) (string, error) {
	parsed, err := ParseKey(threadKey)
	if err != nil {
		return "", fmt.Errorf("invalid thread key: %w", err)
	}
	if parsed.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", parsed.Type)
	}
	return fmt.Sprintf(ThreadMessageGEPrefix, threadKey, PadSeq(seq)), nil
}

func GenUserThreadRelPrefix(userID string) (string, error) {
	// Simple user ID validation - non-empty and reasonable length
	if userID == "" || len(userID) > 256 {
		return "", fmt.Errorf("invalid user ID: %q", userID)
	}
	return fmt.Sprintf(UserThreadRelPrefix, userID), nil
}

func GenThreadUserRelPrefix(threadKey string) (string, error) {
	parsed, err := ParseKey(threadKey)
	if err != nil {
		return "", fmt.Errorf("invalid thread key: %w", err)
	}
	if parsed.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", parsed.Type)
	}
	return fmt.Sprintf(ThreadUserRelPrefix, threadKey), nil
}

func GenSoftDeletePrefix() string {
	return SoftDeletePrefix
}

func ExtractThreadKeyFromMessage(messageKey string) (string, error) {
	parsed, err := ParseKey(messageKey)
	if err != nil {
		return "", fmt.Errorf("invalid message key: %w", err)
	}
	if parsed.Type != KeyTypeMessage && parsed.Type != KeyTypeMessageProvisional {
		return "", fmt.Errorf("expected message key, got %s", parsed.Type)
	}
	return parsed.ThreadTS, nil
}

func ExtractMessageKeyFromVersion(versionKey string) (string, error) {
	parsed, err := ParseKey(versionKey)
	if err != nil {
		return "", fmt.Errorf("invalid version key: %w", err)
	}
	if parsed.Type != KeyTypeVersion {
		return "", fmt.Errorf("expected version key, got %s", parsed.Type)
	}
	threadTS := parsed.ThreadTS
	messageTS := parsed.MessageTS
	return fmt.Sprintf("t:%s:m:%s", threadTS, messageTS), nil
}

func IsThreadKey(key string) bool {
	parsed, err := ParseKey(key)
	return err == nil && parsed.Type == KeyTypeThread
}

func IsMessageKey(key string) bool {
	parsed, err := ParseKey(key)
	return err == nil && (parsed.Type == KeyTypeMessage || parsed.Type == KeyTypeMessageProvisional)
}

func IsVersionKey(key string) bool {
	parsed, err := ParseKey(key)
	return err == nil && parsed.Type == KeyTypeVersion
}

func IsRelationKey(key string) bool {
	parsed, err := ParseKey(key)
	return err == nil && (parsed.Type == KeyTypeUserOwnsThread || parsed.Type == KeyTypeThreadHasUser)
}

func IsIndexKey(key string) bool {
	parsed, err := ParseKey(key)
	if err != nil {
		return false
	}
	switch parsed.Type {
	case KeyTypeThreadMessageStart, KeyTypeThreadMessageEnd, KeyTypeThreadMessageLC, KeyTypeThreadMessageLU,
		KeyTypeDeletedThreadsIndex, KeyTypeDeletedMessagesIndex:
		return true
	default:
		return false
	}
}

func GetKeyType(key string) KeyType {
	parsed, err := ParseKey(key)
	if err != nil {
		return ""
	}
	return parsed.Type
}

func NormalizeKey(key string) (string, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return "", fmt.Errorf("invalid key: %w", err)
	}
	switch parsed.Type {
	case KeyTypeThread:
		return fmt.Sprintf("t:%s", parsed.ThreadTS), nil
	case KeyTypeMessage:
		return fmt.Sprintf("t:%s:m:%s:%s",
			parsed.ThreadTS,
			parsed.MessageTS,
			parsed.Seq), nil
	case KeyTypeMessageProvisional:
		return fmt.Sprintf("t:%s:m:%s",
			parsed.ThreadTS,
			parsed.MessageTS), nil
	case KeyTypeVersion:
		return fmt.Sprintf("v:%s:m:%s",
			parsed.ThreadTS,
			parsed.MessageTS), nil
	case KeyTypeUserOwnsThread:
		return fmt.Sprintf("rel:u:%s:t:%s",
			parsed.UserID,
			parsed.ThreadTS), nil
	case KeyTypeThreadHasUser:
		return fmt.Sprintf("rel:t:%s:u:%s",
			parsed.ThreadTS,
			parsed.UserID), nil
	}
	return key, nil
}

// GenThreadIndexPrefix generates the prefix for all thread-related indexes
func GenThreadIndexPrefix(threadKey string) (string, error) {
	parsed, err := ParseKey(threadKey)
	if err != nil {
		return "", fmt.Errorf("invalid thread key: %w", err)
	}
	if parsed.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", parsed.Type)
	}
	return fmt.Sprintf("idx:t:%s:", parsed.ThreadTS), nil
}
