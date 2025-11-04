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
	result := ValidateKey(messageKey)
	if !result.Valid {
		return "", fmt.Errorf("invalid message key: %w", result.Error)
	}
	if result.Type != KeyTypeMessage && result.Type != KeyTypeMessageProvisional {
		return "", fmt.Errorf("expected message key, got %s", result.Type)
	}
	return fmt.Sprintf(VersionPrefix, messageKey), nil
}

func GenAllThreadMessagesPrefix(threadKey string) (string, error) {
	result, err := ParseKey(threadKey)
	if err != nil {
		return "", fmt.Errorf("invalid thread key: %w", err)
	}
	if result.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", result.Type)
	}
	return fmt.Sprintf(ThreadMessagePrefix, result.ThreadTS), nil
}

func GenThreadMetadataPrefix() string {
	return ThreadMetadataPrefix
}

func GenThreadMessagesGEPrefix(threadKey string, seq uint64) (string, error) {
	result := ValidateKey(threadKey)
	if !result.Valid {
		return "", fmt.Errorf("invalid thread key: %w", result.Error)
	}
	if result.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", result.Type)
	}
	return fmt.Sprintf(ThreadMessageGEPrefix, threadKey, PadSeq(seq)), nil
}

func GenUserThreadRelPrefix(userID string) (string, error) {
	if err := ValidateUserID(userID); err != nil {
		return "", fmt.Errorf("invalid user ID: %w", err)
	}
	return fmt.Sprintf(UserThreadRelPrefix, userID), nil
}

func GenThreadUserRelPrefix(threadKey string) (string, error) {
	result := ValidateKey(threadKey)
	if !result.Valid {
		return "", fmt.Errorf("invalid thread key: %w", result.Error)
	}
	if result.Type != KeyTypeThread {
		return "", fmt.Errorf("expected thread key, got %s", result.Type)
	}
	return fmt.Sprintf(ThreadUserRelPrefix, threadKey), nil
}

func GenSoftDeletePrefix() string {
	return SoftDeletePrefix
}

// Legacy functions which panic on error (backward-compatibility)

func GenAllMessageVersionsPrefixLegacy(messageKey string) string {
	prefix, err := GenAllMessageVersionsPrefix(messageKey)
	if err != nil {
		panic(fmt.Sprintf("GenAllMessageVersionsPrefix: %v", err))
	}
	return prefix
}

func GenAllThreadMessagesPrefixLegacy(threadKey string) string {
	prefix, err := GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		panic(fmt.Sprintf("GenAllThreadMessagesPrefix: %v", err))
	}
	return prefix
}

func GenThreadMessagesGEPrefixLegacy(threadKey string, seq uint64) string {
	prefix, err := GenThreadMessagesGEPrefix(threadKey, seq)
	if err != nil {
		panic(fmt.Sprintf("GenThreadMessagesGEPrefix: %v", err))
	}
	return prefix
}

func GenUserThreadRelPrefixLegacy(userID string) string {
	prefix, err := GenUserThreadRelPrefix(userID)
	if err != nil {
		panic(fmt.Sprintf("GenUserThreadRelPrefix: %v", err))
	}
	return prefix
}

func GenThreadUserRelPrefixLegacy(threadKey string) string {
	prefix, err := GenThreadUserRelPrefix(threadKey)
	if err != nil {
		panic(fmt.Sprintf("GenThreadUserRelPrefix: %v", err))
	}
	return prefix
}

// Key validation and parsing helpers

func ExtractThreadKeyFromMessage(messageKey string) (string, error) {
	result := ValidateKey(messageKey)
	if !result.Valid {
		return "", fmt.Errorf("invalid message key: %w", result.Error)
	}
	if result.Type != KeyTypeMessage && result.Type != KeyTypeMessageProvisional {
		return "", fmt.Errorf("expected message key, got %s", result.Type)
	}
	if result.Parsed == nil {
		return "", fmt.Errorf("failed to parse message key")
	}
	return result.Parsed.ThreadTS, nil
}

func ExtractMessageKeyFromVersion(versionKey string) (string, error) {
	result := ValidateKey(versionKey)
	if !result.Valid {
		return "", fmt.Errorf("invalid version key: %w", result.Error)
	}
	if result.Type != KeyTypeVersion {
		return "", fmt.Errorf("expected version key, got %s", result.Type)
	}
	if result.Parsed == nil {
		return "", fmt.Errorf("failed to parse version key")
	}
	threadTS := result.Parsed.ThreadTS
	messageTS := result.Parsed.MessageTS
	return fmt.Sprintf("t:%s:m:%s", threadTS, messageTS), nil
}

func IsThreadKey(key string) bool {
	result := ValidateKey(key)
	return result.Valid && result.Type == KeyTypeThread
}

func IsMessageKey(key string) bool {
	result := ValidateKey(key)
	return result.Valid && (result.Type == KeyTypeMessage || result.Type == KeyTypeMessageProvisional)
}

func IsVersionKey(key string) bool {
	result := ValidateKey(key)
	return result.Valid && result.Type == KeyTypeVersion
}

func IsRelationKey(key string) bool {
	result := ValidateKey(key)
	return result.Valid && (result.Type == KeyTypeUserOwnsThread || result.Type == KeyTypeThreadHasUser)
}

func IsIndexKey(key string) bool {
	result := ValidateKey(key)
	return result.Valid && (result.Type == KeyTypeThreadMessageStart ||
		result.Type == KeyTypeThreadMessageEnd ||
		result.Type == KeyTypeThreadMessageLC ||
		result.Type == KeyTypeThreadMessageLU ||
		result.Type == KeyTypeThreadMessageCDeltas ||
		result.Type == KeyTypeThreadMessageUDeltas ||
		result.Type == KeyTypeThreadMessageSkips ||
		result.Type == KeyTypeThreadVersionStart ||
		result.Type == KeyTypeThreadVersionEnd ||
		result.Type == KeyTypeThreadVersionLC ||
		result.Type == KeyTypeThreadVersionLU ||
		result.Type == KeyTypeThreadVersionCDeltas ||
		result.Type == KeyTypeThreadVersionUDeltas ||
		result.Type == KeyTypeThreadVersionSkips ||
		result.Type == KeyTypeDeletedThreadsIndex ||
		result.Type == KeyTypeDeletedMessagesIndex)
}

func GetKeyType(key string) KeyType {
	result := ValidateKey(key)
	if !result.Valid {
		return ""
	}
	return result.Type
}

func NormalizeKey(key string) (string, error) {
	result := ValidateKey(key)
	if !result.Valid {
		return "", fmt.Errorf("invalid key: %w", result.Error)
	}
	if result.Type == "simple" {
		return key, nil
	}
	switch result.Type {
	case KeyTypeThread:
		if result.Parsed != nil {
			return fmt.Sprintf("t:%s", result.Parsed.ThreadTS), nil
		}
	case KeyTypeMessage:
		if result.Parsed != nil {
			return fmt.Sprintf("t:%s:m:%s:%s",
				result.Parsed.ThreadTS,
				result.Parsed.MessageTS,
				result.Parsed.Seq), nil
		}
	case KeyTypeMessageProvisional:
		if result.Parsed != nil {
			return fmt.Sprintf("t:%s:m:%s",
				result.Parsed.ThreadTS,
				result.Parsed.MessageTS), nil
		}
	case KeyTypeVersion:
		if result.Parsed != nil {
			return fmt.Sprintf("v:%s:m:%s",
				result.Parsed.ThreadTS,
				result.Parsed.MessageTS), nil
		}
	case KeyTypeUserOwnsThread:
		if result.Parsed != nil {
			return fmt.Sprintf("rel:u:%s:t:%s",
				result.Parsed.UserID,
				result.Parsed.ThreadTS), nil
		}
	case KeyTypeThreadHasUser:
		if result.Parsed != nil {
			return fmt.Sprintf("rel:t:%s:u:%s",
				result.Parsed.ThreadTS,
				result.Parsed.UserID), nil
		}
	}
	return key, nil
}
