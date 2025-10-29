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
	if parts, err := ParseThreadKey(threadKey); err == nil {
		threadComp = parts.ThreadKey
	} else {
		return "", "", fmt.Errorf("extract thread component: %w", err)
	}

	if parts, err := ParseMessageProvisionalKey(messageKey); err == nil {
		messageComp = parts.MessageKey
	} else if IsProvisionalMessageKey(messageKey) {
		messageComp = messageKey
	} else {
		return "", "", fmt.Errorf("extract message component: %w", err)
	}

	return threadComp, messageComp, nil
}
