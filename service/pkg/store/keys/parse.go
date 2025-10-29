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
type ThreadVersionLUIndexParts struct {
	ThreadKey  string
	MessageKey string
}
type ThreadVersionLCIndexParts struct {
	ThreadKey  string
	MessageKey string
}

type UserOwnsThreadParts struct {
	UserID    string
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
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "t" || parts[2] != "m" {
		return nil, fmt.Errorf("invalid message storage key: %s", key)
	}
	threadKey := parts[1]
	messageKey := parts[3]
	seq := parts[4]
	return &MessageKeyParts{
		ThreadKey:  threadKey,
		MessageKey: messageKey,
		Seq:        seq,
	}, nil
}

func ParseVersionKey(key string) (*VersionKeyParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 4 || parts[0] != "v" {
		return nil, fmt.Errorf("invalid version key: %s", key)
	}
	messageKey := parts[1]
	messageTS := parts[2]
	seq := parts[3]
	return &VersionKeyParts{
		MessageKey: messageKey,
		MessageTS:  messageTS,
		Seq:        seq,
	}, nil
}

func ParseThreadKey(key string) (*ThreadMetaParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 2 || parts[0] != "t" {
		return nil, fmt.Errorf("invalid thread key: %s", key)
	}
	return &ThreadMetaParts{ThreadKey: parts[1]}, nil
}

func ParseMessageProvisionalKey(key string) (*MessageKeyParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 4 || parts[0] != "t" || parts[2] != "m" {
		return nil, fmt.Errorf("invalid provisional message key: %s", key)
	}
	threadKey := parts[1]
	messageKey := parts[3]
	return &MessageKeyParts{
		ThreadKey:  threadKey,
		MessageKey: messageKey,
		Seq:        "",
	}, nil
}

func ParseDeletedThreadsIndex(key string) (*DeletedThreadsIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 6 || parts[0] != "idx" || parts[1] != "t" || parts[2] != "deleted" || parts[3] != "u" || parts[5] != "list" {
		return nil, fmt.Errorf("invalid deleted threads index key: %s", key)
	}
	return &DeletedThreadsIndexParts{UserID: parts[4]}, nil
}

func ParseDeletedMessagesIndex(key string) (*DeletedMessagesIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 6 || parts[0] != "idx" || parts[1] != "m" || parts[2] != "deleted" || parts[3] != "u" || parts[5] != "list" {
		return nil, fmt.Errorf("invalid deleted messages index key: %s", key)
	}
	return &DeletedMessagesIndexParts{UserID: parts[4]}, nil
}

func ParseThreadMessageStart(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "start" {
		return "", fmt.Errorf("invalid thread message start key: %s", key)
	}
	return parts[2], nil
}

func ParseThreadMessageEnd(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "end" {
		return "", fmt.Errorf("invalid thread message end key: %s", key)
	}
	return parts[2], nil
}

func ParseThreadMessageCDeltas(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "cdeltas" {
		return "", fmt.Errorf("invalid thread message cdeltas key: %s", key)
	}
	return parts[2], nil
}

func ParseThreadMessageUDeltas(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "udeltas" {
		return "", fmt.Errorf("invalid thread message udeltas key: %s", key)
	}
	return parts[2], nil
}

func ParseThreadMessageSkips(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "skips" {
		return "", fmt.Errorf("invalid thread message skips key: %s", key)
	}
	return parts[2], nil
}

func ParseThreadMessageLC(key string) (*ThreadMessageLCIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "lc" {
		return nil, fmt.Errorf("invalid thread message lc key: %s", key)
	}
	return &ThreadMessageLCIndexParts{ThreadKey: parts[2]}, nil
}

func ParseThreadMessageLU(key string) (*ThreadMessageLUIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "lu" {
		return nil, fmt.Errorf("invalid thread message lu key: %s", key)
	}
	return &ThreadMessageLUIndexParts{ThreadKey: parts[2]}, nil
}

func ParseThreadVersionStart(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "start" {
		return "", "", fmt.Errorf("invalid thread version start key: %s", key)
	}
	return parts[2], parts[4], nil
}

func ParseThreadVersionEnd(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "end" {
		return "", "", fmt.Errorf("invalid thread version end key: %s", key)
	}
	return parts[2], parts[4], nil
}

func ParseThreadVersionCDeltas(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "cdeltas" {
		return "", "", fmt.Errorf("invalid thread version cdeltas key: %s", key)
	}
	return parts[2], parts[4], nil
}

func ParseThreadVersionUDeltas(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "udeltas" {
		return "", "", fmt.Errorf("invalid thread version udeltas key: %s", key)
	}
	return parts[2], parts[4], nil
}

func ParseThreadVersionSkips(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "skips" {
		return "", "", fmt.Errorf("invalid thread version skips key: %s", key)
	}
	return parts[2], parts[4], nil
}

func ParseThreadVersionLC(key string) (*ThreadVersionLCIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "vs" || parts[6] != "lc" {
		return nil, fmt.Errorf("invalid thread version lc key: %s", key)
	}
	return &ThreadVersionLCIndexParts{ThreadKey: parts[2], MessageKey: parts[4]}, nil
}

func ParseThreadVersionLU(key string) (*ThreadVersionLUIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "vs" || parts[6] != "lu" {
		return nil, fmt.Errorf("invalid thread version lu key: %s", key)
	}
	return &ThreadVersionLUIndexParts{ThreadKey: parts[2], MessageKey: parts[4]}, nil
}

func ParseUserOwnsThread(key string) (*UserOwnsThreadParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "rel" || parts[1] != "u" || parts[3] != "t" {
		return nil, fmt.Errorf("invalid user owns thread key: %s", key)
	}
	userID := parts[2]
	// Include the "t:" prefix for thread key as required (i.e., parts[3] + ":" + parts[4])
	threadKey := parts[3] + ":" + parts[4]
	return &UserOwnsThreadParts{
		UserID:    userID,
		ThreadKey: threadKey,
	}, nil
}

func ParseKeyTimestamp(s string) (int64, error) {
	return parsePaddedInt(s, TSPadWidth)
}

func ParseKeySequence(s string) (uint64, error) {
	return parsePaddedUint(s, SeqPadWidth)
}

func ExtractMessageComponents(threadKey, messageKey string) (threadComp, messageComp string, err error) {
	if parsed, err := ParseKey(threadKey); err == nil && parsed.Type == KeyTypeThread {
		threadComp = parsed.ThreadTS
	} else {
		return "", "", fmt.Errorf("extract thread component: %w", err)
	}

	if parsed, err := ParseKey(messageKey); err == nil && (parsed.Type == KeyTypeMessageProvisional || parsed.Type == KeyTypeMessage) {
		messageComp = parsed.MessageTS
	} else if IsProvisionalMessageKey(messageKey) {
		messageComp = messageKey
	} else {
		return "", "", fmt.Errorf("extract message component: %w", err)
	}

	return threadComp, messageComp, nil
}

// KeyType represents the type of key
type KeyType string

const (
	KeyTypeThread               KeyType = "thread"
	KeyTypeMessage              KeyType = "message"
	KeyTypeMessageProvisional   KeyType = "message_provisional"
	KeyTypeVersion              KeyType = "version"
	KeyTypeUserOwnsThread       KeyType = "user_owns_thread"
	KeyTypeThreadMessageStart   KeyType = "thread_message_start"
	KeyTypeThreadMessageEnd     KeyType = "thread_message_end"
	KeyTypeThreadMessageLC      KeyType = "thread_message_lc"
	KeyTypeThreadMessageLU      KeyType = "thread_message_lu"
	KeyTypeThreadVersionStart   KeyType = "thread_version_start"
	KeyTypeThreadVersionEnd     KeyType = "thread_version_end"
	KeyTypeThreadVersionLC      KeyType = "thread_version_lc"
	KeyTypeThreadVersionLU      KeyType = "thread_version_lu"
	KeyTypeDeletedThreadsIndex  KeyType = "deleted_threads_index"
	KeyTypeDeletedMessagesIndex KeyType = "deleted_messages_index"
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
		MessageTS: parts[1],
		Seq:       parts[3],
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

	// idx:t:{threadTS}:ms:{messageTS}:v:{type}
	if len(parts) >= 7 && parts[1] == "t" && parts[3] == "ms" && parts[5] == "v" {
		threadTS := parts[2]
		messageTS := parts[4]
		fullThreadKey := "t:" + threadTS
		indexType := parts[6]

		var keyType KeyType
		switch indexType {
		case "start":
			keyType = KeyTypeThreadVersionStart
		case "end":
			keyType = KeyTypeThreadVersionEnd
		case "lc":
			keyType = KeyTypeThreadVersionLC
		case "lu":
			keyType = KeyTypeThreadVersionLU
		default:
			return nil, fmt.Errorf("unknown thread version index type: %s", indexType)
		}

		return &KeyParts{
			Type:      keyType,
			ThreadKey: fullThreadKey,
			ThreadTS:  threadTS,
			MessageTS: messageTS,
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
