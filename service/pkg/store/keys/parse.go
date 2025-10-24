package keys

import (
	"fmt"
	"strconv"
	"strings"
)

// Struct types for returns

type MessageKeyParts struct {
	ThreadID string
	MsgID    string
	Seq      string
}

type VersionKeyParts struct {
	MsgID string
	TS    string
	Seq   string
}

type ThreadMetaParts struct {
	ThreadID string
}

type ThreadIDParts struct {
	Hex       string
	Timestamp int64
}

type UserThreadsIndexParts struct {
	UserID string
}

type ThreadParticipantsIndexParts struct {
	ThreadID string
}

type DeletedThreadsIndexParts struct {
	UserID string
}

type DeletedMessagesIndexParts struct {
	UserID string
}

// --- New Struct Types for LU/CU Index Parsers ---

type ThreadMessageLUIndexParts struct {
	ThreadID string
}
type ThreadMessageLCIndexParts struct {
	ThreadID string
}
type ThreadVersionLUIndexParts struct {
	ThreadID string
	MsgID    string
}
type ThreadVersionLCIndexParts struct {
	ThreadID string
	MsgID    string
}

// -----------------
// Padding Utilities
// -----------------

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

// ------------
// Key Parsers
// ------------

// ParseMessageKey parses keys formatted as t:<threadID>:m:<msgID>:<seq>
func ParseMessageKey(key string) (*MessageKeyParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "t" || parts[2] != "m" {
		return nil, fmt.Errorf("invalid message storage key: %s", key)
	}
	threadID := parts[1]
	msgID := parts[3]
	seq := parts[4]
	return &MessageKeyParts{
		ThreadID: threadID,
		MsgID:    msgID,
		Seq:      seq,
	}, nil
}

// ParseVersionKey parses keys formatted as v:<msgID>:<ts>:<seq>
func ParseVersionKey(key string) (*VersionKeyParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 4 || parts[0] != "v" {
		return nil, fmt.Errorf("invalid version key: %s", key)
	}
	msgID := parts[1]
	ts := parts[2]
	seq := parts[3]
	return &VersionKeyParts{
		MsgID: msgID,
		TS:    ts,
		Seq:   seq,
	}, nil
}

// ParseThreadMeta parses keys formatted as t:<threadID>:meta
func ParseThreadMeta(key string) (*ThreadMetaParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 || parts[0] != "t" || parts[2] != "meta" {
		return nil, fmt.Errorf("invalid thread meta key: %s", key)
	}
	return &ThreadMetaParts{ThreadID: parts[1]}, nil
}

// ParseThreadID parses thread IDs formatted as thread-{hex}-{timestamp}
func ParseThreadID(threadID string) (*ThreadIDParts, error) {
	parts := strings.Split(threadID, "-")
	if len(parts) != 3 || parts[0] != "thread" {
		return nil, fmt.Errorf("invalid thread ID format: %s", threadID)
	}

	timestamp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp in thread ID %s: %w", threadID, err)
	}

	return &ThreadIDParts{
		Hex:       parts[1],
		Timestamp: timestamp,
	}, nil
}

// ParseUserThreadsIndex parses keys formatted as idx:u:<user_id>:threads
func ParseUserThreadsIndex(key string) (*UserThreadsIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 4 || parts[0] != "idx" || parts[1] != "u" || parts[3] != "threads" {
		return nil, fmt.Errorf("invalid user threads index key: %s", key)
	}
	return &UserThreadsIndexParts{UserID: parts[2]}, nil
}

// ParseThreadParticipantsIndex parses keys formatted as idx:p:<thread_id>
func ParseThreadParticipantsIndex(key string) (*ThreadParticipantsIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 || parts[0] != "idx" || parts[1] != "p" {
		return nil, fmt.Errorf("invalid thread participants index key: %s", key)
	}
	return &ThreadParticipantsIndexParts{ThreadID: parts[2]}, nil
}

// ParseDeletedThreadsIndex parses keys formatted as idx:t:deleted:u:<user_id>:list
func ParseDeletedThreadsIndex(key string) (*DeletedThreadsIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 6 || parts[0] != "idx" || parts[1] != "t" || parts[2] != "deleted" || parts[3] != "u" || parts[5] != "list" {
		return nil, fmt.Errorf("invalid deleted threads index key: %s", key)
	}
	return &DeletedThreadsIndexParts{UserID: parts[4]}, nil
}

// ParseDeletedMessagesIndex parses keys formatted as idx:m:deleted:u:<user_id>:list
func ParseDeletedMessagesIndex(key string) (*DeletedMessagesIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 6 || parts[0] != "idx" || parts[1] != "m" || parts[2] != "deleted" || parts[3] != "u" || parts[5] != "list" {
		return nil, fmt.Errorf("invalid deleted messages index key: %s", key)
	}
	return &DeletedMessagesIndexParts{UserID: parts[4]}, nil
}

// --- Additional Parsers for Thread/Message Indexes ---

// ParseThreadMessageStart parses keys formatted as idx:t:<thread_id>:ms:start
func ParseThreadMessageStart(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "start" {
		return "", fmt.Errorf("invalid thread message start key: %s", key)
	}
	return parts[2], nil
}

// ParseThreadMessageEnd parses keys formatted as idx:t:<thread_id>:ms:end
func ParseThreadMessageEnd(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "end" {
		return "", fmt.Errorf("invalid thread message end key: %s", key)
	}
	return parts[2], nil
}

// ParseThreadMessageCDeltas parses keys formatted as idx:t:<thread_id>:ms:cdeltas
func ParseThreadMessageCDeltas(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "cdeltas" {
		return "", fmt.Errorf("invalid thread message cdeltas key: %s", key)
	}
	return parts[2], nil
}

// ParseThreadMessageUDeltas parses keys formatted as idx:t:<thread_id>:ms:udeltas
func ParseThreadMessageUDeltas(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "udeltas" {
		return "", fmt.Errorf("invalid thread message udeltas key: %s", key)
	}
	return parts[2], nil
}

// ParseThreadMessageSkips parses keys formatted as idx:t:<thread_id>:ms:skips
func ParseThreadMessageSkips(key string) (threadID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "skips" {
		return "", fmt.Errorf("invalid thread message skips key: %s", key)
	}
	return parts[2], nil
}

// ParseThreadMessageLC parses keys formatted as idx:t:<thread_id>:ms:lc
func ParseThreadMessageLC(key string) (*ThreadMessageLCIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "lc" {
		return nil, fmt.Errorf("invalid thread message lc key: %s", key)
	}
	return &ThreadMessageLCIndexParts{ThreadID: parts[2]}, nil
}

// ParseThreadMessageLU parses keys formatted as idx:t:<thread_id>:ms:lu
func ParseThreadMessageLU(key string) (*ThreadMessageLUIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 5 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[4] != "lu" {
		return nil, fmt.Errorf("invalid thread message lu key: %s", key)
	}
	return &ThreadMessageLUIndexParts{ThreadID: parts[2]}, nil
}

// ParseThreadVersionStart parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:v:start
func ParseThreadVersionStart(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "start" {
		return "", "", fmt.Errorf("invalid thread version start key: %s", key)
	}
	return parts[2], parts[4], nil
}

// ParseThreadVersionEnd parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:v:end
func ParseThreadVersionEnd(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "end" {
		return "", "", fmt.Errorf("invalid thread version end key: %s", key)
	}
	return parts[2], parts[4], nil
}

// ParseThreadVersionCDeltas parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:v:cdeltas
func ParseThreadVersionCDeltas(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "cdeltas" {
		return "", "", fmt.Errorf("invalid thread version cdeltas key: %s", key)
	}
	return parts[2], parts[4], nil
}

// ParseThreadVersionUDeltas parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:v:udeltas
func ParseThreadVersionUDeltas(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "udeltas" {
		return "", "", fmt.Errorf("invalid thread version udeltas key: %s", key)
	}
	return parts[2], parts[4], nil
}

// ParseThreadVersionSkips parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:v:skips
func ParseThreadVersionSkips(key string) (threadID, msgID string, err error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "v" || parts[6] != "skips" {
		return "", "", fmt.Errorf("invalid thread version skips key: %s", key)
	}
	return parts[2], parts[4], nil
}

// ParseThreadVersionLC parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:vs:lc
func ParseThreadVersionLC(key string) (*ThreadVersionLCIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "vs" || parts[6] != "lc" {
		return nil, fmt.Errorf("invalid thread version lc key: %s", key)
	}
	return &ThreadVersionLCIndexParts{ThreadID: parts[2], MsgID: parts[4]}, nil
}

// ParseThreadVersionLU parses keys formatted as idx:t:<thread_id>:ms:<msg_id>:vs:lu
func ParseThreadVersionLU(key string) (*ThreadVersionLUIndexParts, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "idx" || parts[1] != "t" || parts[3] != "ms" || parts[5] != "vs" || parts[6] != "lu" {
		return nil, fmt.Errorf("invalid thread version lu key: %s", key)
	}
	return &ThreadVersionLUIndexParts{ThreadID: parts[2], MsgID: parts[4]}, nil
}

// -----------------------
// Utility Parsers for TS/Seq in string
// -----------------------

func ParseKeyTimestamp(s string) (int64, error) {
	return parsePaddedInt(s, TSPadWidth)
}

func ParseKeySequence(s string) (uint64, error) {
	return parsePaddedUint(s, SeqPadWidth)
}
