package keys

import (
	"fmt"
	"strconv"
	"strings"
)

type MessageKeyParts struct {
	ThreadKey  string
	MessageKey string
	Seq        string
}

type VersionKeyParts struct {
	MessageKey string
	MessageTS  string
	Seq        string
}

type ThreadMetaParts struct {
	ThreadKey string
}

type DeletedThreadsIndexParts struct {
	UserID string
}

type DeletedMessagesIndexParts struct {
	UserID string
}

type ThreadMessageLUIndexParts struct {
	ThreadKey string
}

type ThreadMessageLCIndexParts struct {
	ThreadKey string
}

type UserOwnsThreadParts struct {
	UserID    string
	ThreadKey string
}

type ThreadHasUserParts struct {
	ThreadKey string
	UserID    string
}

type SoftDeleteMarkerParts struct {
	OriginalKey string
}

type ThreadMessageStartParts struct {
	ThreadKey string
}

type ThreadMessageEndParts struct {
	ThreadKey string
}

type ThreadMessageCDeltasParts struct {
	ThreadKey string
}

type ThreadMessageUDeltasParts struct {
	ThreadKey string
}

type ThreadMessageSkipsParts struct {
	ThreadKey string
}

func parsePaddedInt(s string, width int) (int64, error) {
	if len(s) == 0 || len(s) > width {
		return 0, fmt.Errorf("length invalid: %s", s)
	}
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func parsePaddedUint(s string, width int) (uint64, error) {
	if len(s) == 0 || len(s) > width {
		return 0, fmt.Errorf("length invalid: %s", s)
	}
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func ParseMessageKey(key string) (*MessageKeyParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeMessage {
		return nil, fmt.Errorf("expected message key, got %s", parsed.Type)
	}
	return &MessageKeyParts{
		ThreadKey:  parsed.ThreadKey,
		MessageKey: parsed.MessageKey,
		Seq:        parsed.Seq,
	}, nil
}

func ParseVersionKey(key string) (*VersionKeyParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeVersion {
		return nil, fmt.Errorf("expected version key, got %s", parsed.Type)
	}
	return &VersionKeyParts{
		MessageKey: parsed.MessageTS, // messageKey from v:{messageKey}:{ts}:{seq}
		MessageTS:  parsed.VersionTS, // timestamp from v:{messageKey}:{ts}:{seq}
		Seq:        parsed.Seq,       // seq from v:{messageKey}:{ts}:{seq}
	}, nil
}

func ParseThreadKey(key string) (*ThreadMetaParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThread {
		return nil, fmt.Errorf("expected thread key, got %s", parsed.Type)
	}
	return &ThreadMetaParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseMessageProvisionalKey(key string) (*MessageKeyParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeMessageProvisional {
		return nil, fmt.Errorf("expected provisional message key, got %s", parsed.Type)
	}
	return &MessageKeyParts{
		ThreadKey:  parsed.ThreadKey,
		MessageKey: parsed.MessageProvKey,
		Seq:        "",
	}, nil
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

func ParseDeletedThreadsIndex(key string) (*DeletedThreadsIndexParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeDeletedThreadsIndex {
		return nil, fmt.Errorf("expected deleted threads index key, got %s", parsed.Type)
	}
	return &DeletedThreadsIndexParts{UserID: parsed.UserID}, nil
}

func ParseDeletedMessagesIndex(key string) (*DeletedMessagesIndexParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeDeletedMessagesIndex {
		return nil, fmt.Errorf("expected deleted messages index key, got %s", parsed.Type)
	}
	return &DeletedMessagesIndexParts{UserID: parsed.UserID}, nil
}

func ParseThreadMessageStart(key string) (*ThreadMessageStartParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageStart {
		return nil, fmt.Errorf("expected thread message start key, got %s", parsed.Type)
	}
	return &ThreadMessageStartParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseThreadMessageEnd(key string) (*ThreadMessageEndParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageEnd {
		return nil, fmt.Errorf("expected thread message end key, got %s", parsed.Type)
	}
	return &ThreadMessageEndParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseThreadMessageCDeltas(key string) (*ThreadMessageCDeltasParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageCDeltas {
		return nil, fmt.Errorf("expected thread message cdeltas key, got %s", parsed.Type)
	}
	return &ThreadMessageCDeltasParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseThreadMessageUDeltas(key string) (*ThreadMessageUDeltasParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageUDeltas {
		return nil, fmt.Errorf("expected thread message udeltas key, got %s", parsed.Type)
	}
	return &ThreadMessageUDeltasParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseThreadMessageSkips(key string) (*ThreadMessageSkipsParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageSkips {
		return nil, fmt.Errorf("expected thread message skips key, got %s", parsed.Type)
	}
	return &ThreadMessageSkipsParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseThreadMessageLC(key string) (*ThreadMessageLCIndexParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageLC {
		return nil, fmt.Errorf("expected thread message lc key, got %s", parsed.Type)
	}
	return &ThreadMessageLCIndexParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseThreadMessageLU(key string) (*ThreadMessageLUIndexParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadMessageLU {
		return nil, fmt.Errorf("expected thread message lu key, got %s", parsed.Type)
	}
	return &ThreadMessageLUIndexParts{ThreadKey: parsed.ThreadKey}, nil
}

func ParseUserOwnsThread(key string) (*UserOwnsThreadParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeUserOwnsThread {
		return nil, fmt.Errorf("expected user owns thread key, got %s", parsed.Type)
	}
	return &UserOwnsThreadParts{
		UserID:    parsed.UserID,
		ThreadKey: parsed.ThreadKey,
	}, nil
}

func ParseThreadHasUser(key string) (*ThreadHasUserParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeThreadHasUser {
		return nil, fmt.Errorf("expected thread has user key, got %s", parsed.Type)
	}
	return &ThreadHasUserParts{
		ThreadKey: parsed.ThreadKey,
		UserID:    parsed.UserID,
	}, nil
}

func ParseSoftDeleteMarker(key string) (*SoftDeleteMarkerParts, error) {
	parsed, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if parsed.Type != KeyTypeSoftDeleteMarker {
		return nil, fmt.Errorf("expected soft delete marker key, got %s", parsed.Type)
	}

	// Extract original key by removing "del:" prefix
	originalKey := strings.TrimPrefix(key, "del:")
	return &SoftDeleteMarkerParts{
		OriginalKey: originalKey,
	}, nil
}

func ParseKeyTimestamp(s string) (int64, error) {
	return parsePaddedInt(s, TSPadWidth)
}

func ParseKeySequence(s string) (uint64, error) {
	return parsePaddedUint(s, SeqPadWidth)
}

// IsProvisionalMessageKey checks if a key is a provisional message key
// Provisional keys have format: t:{threadTS}:m:{messageTS}
// Final keys have format: t:{threadTS}:m:{messageTS}:{seq}
func IsProvisionalMessageKey(key string) bool {
	parsed, err := ParseKey(key)
	if err != nil {
		return false
	}
	return parsed.Type == KeyTypeMessageProvisional
}

// KeyType represents the type of key
type KeyType string

const (
	KeyTypeThread               KeyType = "thread"
	KeyTypeMessage              KeyType = "message"
	KeyTypeMessageProvisional   KeyType = "message_provisional"
	KeyTypeVersion              KeyType = "version"
	KeyTypeUserOwnsThread       KeyType = "user_owns_thread"
	KeyTypeThreadHasUser        KeyType = "thread_has_user"
	KeyTypeThreadMessageStart   KeyType = "thread_message_start"
	KeyTypeThreadMessageEnd     KeyType = "thread_message_end"
	KeyTypeThreadMessageLC      KeyType = "thread_message_lc"
	KeyTypeThreadMessageLU      KeyType = "thread_message_lu"
	KeyTypeThreadMessageCDeltas KeyType = "thread_message_cdeltas"
	KeyTypeThreadMessageUDeltas KeyType = "thread_message_udeltas"
	KeyTypeThreadMessageSkips   KeyType = "thread_message_skips"

	KeyTypeDeletedThreadsIndex  KeyType = "deleted_threads_index"
	KeyTypeDeletedMessagesIndex KeyType = "deleted_messages_index"
	KeyTypeSoftDeleteMarker     KeyType = "soft_delete_marker"
)

// KeyParts represents the parsed parts of any key
type KeyParts struct {
	Type           KeyType
	ThreadKey      string // Full: "t:1761739879505665000"
	ThreadTS       string // Just: "1761739879505665000"
	MessageKey     string // Full: "t:1761739879505665000:m:msg123:001"
	MessageTS      string // Just: "msg123"
	MessageProvKey string // Provisional: "t:1761739879505665000:m:msg123"
	Seq            string // "001"
	UserID         string // For relationship keys
	IndexType      string // "start", "end", "lc", "lu", etc.
	VersionTS      string // For version keys: timestamp from v:{messageKey}:{ts}:{seq}
	OriginalKey    string // For soft delete markers: original key without "del:" prefix
}

// ParseKey is the unified key parser that can handle all key formats
func ParseKey(key string) (*KeyParts, error) {
	if key == "" {
		return nil, fmt.Errorf("empty key")
	}

	parts := strings.Split(key, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid key format: %s", key)
	}

	// Route to appropriate parser based on prefix
	switch parts[0] {
	case "t":
		return parseThreadBasedKey(key, parts)
	case "v":
		return parseVersionKey(key, parts)
	case "rel":
		return parseRelationKey(key, parts)
	case "idx":
		return parseIndexKey(key, parts)
	case "del":
		return parseSoftDeleteKey(key, parts)
	default:
		return nil, fmt.Errorf("unknown key prefix: %s", parts[0])
	}
}

// parseThreadBasedKey handles keys starting with "t:"
func parseThreadBasedKey(key string, parts []string) (*KeyParts, error) {
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid thread key format: %s", key)
	}

	threadTS := parts[1]
	fullThreadKey := "t:" + threadTS

	// t:{threadTS} - thread metadata
	if len(parts) == 2 {
		return &KeyParts{
			Type:      KeyTypeThread,
			ThreadKey: fullThreadKey,
			ThreadTS:  threadTS,
		}, nil
	}

	// t:{threadTS}:m:{messageTS}[:{seq}] - message keys
	if len(parts) >= 4 && parts[2] == "m" {
		messageTS := parts[3]
		seq := ""
		if len(parts) >= 5 {
			seq = parts[4]
		}

		keyType := KeyTypeMessageProvisional
		if seq != "" {
			keyType = KeyTypeMessage
		}

		if seq != "" {
			// Full message key
			fullMessageKey := fmt.Sprintf("t:%s:m:%s:%s", threadTS, messageTS, seq)
			return &KeyParts{
				Type:       keyType,
				ThreadKey:  fullThreadKey,
				ThreadTS:   threadTS,
				MessageKey: fullMessageKey,
				MessageTS:  messageTS,
				Seq:        seq,
			}, nil
		} else {
			// Provisional message key
			fullProvKey := fmt.Sprintf("t:%s:m:%s", threadTS, messageTS)
			return &KeyParts{
				Type:           keyType,
				ThreadKey:      fullThreadKey,
				ThreadTS:       threadTS,
				MessageProvKey: fullProvKey,
				MessageTS:      messageTS,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid thread-based key format: %s", key)
}

// parseVersionKey handles keys starting with "v:"
func parseVersionKey(key string, parts []string) (*KeyParts, error) {
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid version key format: %s", key)
	}

	return &KeyParts{
		Type:      KeyTypeVersion,
		MessageTS: parts[1], // messageKey from v:{messageKey}:{ts}:{seq}
		VersionTS: parts[2], // timestamp from v:{messageKey}:{ts}:{seq}
		Seq:       parts[3], // seq from v:{messageKey}:{ts}:{seq}
	}, nil
}

// parseRelationKey handles keys starting with "rel:"
func parseRelationKey(key string, parts []string) (*KeyParts, error) {
	// rel:u:{userID}:t:{threadTS}
	if len(parts) == 5 && parts[1] == "u" && parts[3] == "t" {
		threadTS := parts[4]
		fullThreadKey := "t:" + threadTS
		return &KeyParts{
			Type:      KeyTypeUserOwnsThread,
			UserID:    parts[2],
			ThreadKey: fullThreadKey,
			ThreadTS:  threadTS,
		}, nil
	}

	// rel:t:{threadTS}:u:{userID}
	if len(parts) == 5 && parts[1] == "t" && parts[3] == "u" {
		threadTS := parts[2]
		userID := parts[4]
		fullThreadKey := "t:" + threadTS
		return &KeyParts{
			Type:      KeyTypeThreadHasUser,
			ThreadKey: fullThreadKey,
			ThreadTS:  threadTS,
			UserID:    userID,
		}, nil
	}

	return nil, fmt.Errorf("invalid relation key format: %s", key)
}

// parseIndexKey handles keys starting with "idx:"
func parseIndexKey(key string, parts []string) (*KeyParts, error) {
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid index key format: %s", key)
	}

	// idx:t:{threadTS}:ms:{type}
	if parts[1] == "t" && parts[3] == "ms" {
		threadTS := parts[2]
		fullThreadKey := "t:" + threadTS
		indexType := parts[4]

		var keyType KeyType
		switch indexType {
		case "start":
			keyType = KeyTypeThreadMessageStart
		case "end":
			keyType = KeyTypeThreadMessageEnd
		case "lc":
			keyType = KeyTypeThreadMessageLC
		case "lu":
			keyType = KeyTypeThreadMessageLU
		case "cdeltas":
			keyType = KeyTypeThreadMessageCDeltas
		case "udeltas":
			keyType = KeyTypeThreadMessageUDeltas
		case "skips":
			keyType = KeyTypeThreadMessageSkips
		default:
			return nil, fmt.Errorf("unknown thread message index type: %s", indexType)
		}

		return &KeyParts{
			Type:      keyType,
			ThreadKey: fullThreadKey,
			ThreadTS:  threadTS,
			IndexType: indexType,
		}, nil
	}

	// idx:t:deleted:u:{userID}:list
	if len(parts) == 6 && parts[1] == "t" && parts[2] == "deleted" && parts[3] == "u" && parts[5] == "list" {
		return &KeyParts{
			Type:   KeyTypeDeletedThreadsIndex,
			UserID: parts[4],
		}, nil
	}

	// idx:m:deleted:u:{userID}:list
	if len(parts) == 6 && parts[1] == "m" && parts[2] == "deleted" && parts[3] == "u" && parts[5] == "list" {
		return &KeyParts{
			Type:   KeyTypeDeletedMessagesIndex,
			UserID: parts[4],
		}, nil
	}

	return nil, fmt.Errorf("invalid index key format: %s", key)
}

// parseSoftDeleteKey handles keys starting with "del:"
func parseSoftDeleteKey(key string, parts []string) (*KeyParts, error) {
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid soft delete key format: %s", key)
	}

	// Extract original key by removing "del:" prefix
	originalKey := strings.TrimPrefix(key, "del:")
	return &KeyParts{
		Type:        KeyTypeSoftDeleteMarker,
		OriginalKey: originalKey,
	}, nil
}
