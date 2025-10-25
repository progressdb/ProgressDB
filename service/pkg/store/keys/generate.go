package keys

import (
	"fmt"
)

// GenThreadPrvKey returns a provisional thread key: t:<threadID>
func GenThreadPrvKey(threadID string) string {
	return fmt.Sprintf(ThreadPrvKey, threadID)
}

// GenThreadPrvKey returns a provisional thread key: t:<threadID>
func GenMessagePrvKey(threadID string, messageID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(MessagePrvKey, threadID, messageID)
}

// GenMessageKey returns a message key: t:<threadID>:m:<msgID>:<seq>
func GenMessageKey(threadID, msgID string, seq uint64) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	// Extract message ID component from msgID parameter
	if parts, err := ParseMessageKey(msgID); err == nil {
		msgID = parts.MsgID
	} else if parts, err := ParseMessageProvisionalKey(msgID); err == nil {
		msgID = parts.MsgID
	}
	return fmt.Sprintf(MessageKey, threadID, msgID, PadSeq(seq))
}

// GenVersionKey returns a version key: v:<msgID>:<ts>:<seq>
func GenVersionKey(msgID string, ts int64, seq uint64) string {
	return fmt.Sprintf(VersionKey, msgID, PadTS(ts), PadSeq(seq))
}

// GenThreadKey returns a thread meta key: t:<threadID>:meta
func GenThreadKey(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadKey, threadID)
}

// Thread → message indexes
func GenThreadMessageStart(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageStart, threadID)
}
func GenThreadMessageEnd(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageEnd, threadID)
}
func GenThreadMessageCDeltas(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageCDeltas, threadID)
}
func GenThreadMessageUDeltas(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageUDeltas, threadID)
}
func GenThreadMessageSkips(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageSkips, threadID)
}
func GenThreadMessageLC(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageLC, threadID)
}
func GenThreadMessageLU(threadID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadMessageLU, threadID)
}

// Thread → message version indexes
func GenThreadVersionStart(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionStart, threadID, msgID)
}
func GenThreadVersionEnd(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionEnd, threadID, msgID)
}
func GenThreadVersionCDeltas(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionCDeltas, threadID, msgID)
}
func GenThreadVersionUDeltas(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionUDeltas, threadID, msgID)
}
func GenThreadVersionSkips(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionSkips, threadID, msgID)
}
func GenThreadVersionLC(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionLC, threadID, msgID)
}
func GenThreadVersionLU(threadID, msgID string) string {
	if parts, err := ParseThreadKey(threadID); err == nil {
		threadID = parts.ThreadID
	}
	return fmt.Sprintf(ThreadVersionLU, threadID, msgID)
}

// Soft delete markers
func GenSoftDeleteMarkerKey(originalKey string) string {
	return fmt.Sprintf(SoftDeleteMarker, originalKey)
}

// Relationship markers
func GenUserOwnsThreadKey(userID, threadID string) string {
	return fmt.Sprintf(RelUserOwnsThread, userID, threadID)
}

func GenThreadHasUserKey(threadID, userID string) string {
	return fmt.Sprintf(RelThreadHasUser, threadID, userID)
}

// PadTS returns timestamp padded for key (20 width, lexicographic sort)
func PadTS(ts int64) string {
	return fmt.Sprintf("%0*d", TSPadWidth, ts)
}

// PadSeq returns sequence padded for key (6 width, lexicographic sort)
func PadSeq(seq uint64) string {
	return fmt.Sprintf("%0*d", SeqPadWidth, seq)
}
