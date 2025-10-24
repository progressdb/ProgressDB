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
	return fmt.Sprintf(MessagePrvKey, threadID, messageID)
}

// GenMessageKey returns a message key: t:<threadID>:m:<msgID>:<seq>
func GenMessageKey(threadID, msgID string, seq uint64) string {
	return fmt.Sprintf(MessageKey, threadID, msgID, PadSeq(seq))
}

// GenVersionKey returns a version key: v:<msgID>:<ts>:<seq>
func GenVersionKey(msgID string, ts int64, seq uint64) string {
	return fmt.Sprintf(VersionKey, msgID, PadTS(ts), PadSeq(seq))
}

// GenThreadKey returns a thread meta key: t:<threadID>:meta
func GenThreadKey(threadID string) string {
	return fmt.Sprintf(ThreadKey, threadID)
}

// Thread → message indexes
func GenThreadMessageStart(threadID string) string {
	return fmt.Sprintf(ThreadMessageStart, threadID)
}
func GenThreadMessageEnd(threadID string) string {
	return fmt.Sprintf(ThreadMessageEnd, threadID)
}
func GenThreadMessageCDeltas(threadID string) string {
	return fmt.Sprintf(ThreadMessageCDeltas, threadID)
}
func GenThreadMessageUDeltas(threadID string) string {
	return fmt.Sprintf(ThreadMessageUDeltas, threadID)
}
func GenThreadMessageSkips(threadID string) string {
	return fmt.Sprintf(ThreadMessageSkips, threadID)
}
func GenThreadMessageLC(threadID string) string {
	return fmt.Sprintf(ThreadMessageLC, threadID)
}
func GenThreadMessageLU(threadID string) string {
	return fmt.Sprintf(ThreadMessageLU, threadID)
}

// Thread → message version indexes
func GenThreadVersionStart(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionStart, threadID, msgID)
}
func GenThreadVersionEnd(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionEnd, threadID, msgID)
}
func GenThreadVersionCDeltas(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionCDeltas, threadID, msgID)
}
func GenThreadVersionUDeltas(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionUDeltas, threadID, msgID)
}
func GenThreadVersionSkips(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionSkips, threadID, msgID)
}
func GenThreadVersionLC(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionLC, threadID, msgID)
}
func GenThreadVersionLU(threadID, msgID string) string {
	return fmt.Sprintf(ThreadVersionLU, threadID, msgID)
}

// User → thread index (ownership)
func GenUserThreadsKey(userID string) string {
	return fmt.Sprintf(UserThreads, userID)
}

// Thread → participant indexes
func GenThreadParticipantsKey(threadID string) string {
	return fmt.Sprintf(ThreadParticipants, threadID)
}

// Deletion indexes
func GenDeletedThreadsKey(userID string) string {
	return fmt.Sprintf(DeletedThreads, userID)
}
func GenDeletedMessagesKey(userID string) string {
	return fmt.Sprintf(DeletedMessages, userID)
}

// --- Helper Pad functions ---

// PadTS returns timestamp padded for key (20 width, lexicographic sort)
func PadTS(ts int64) string {
	return fmt.Sprintf("%0*d", TSPadWidth, ts)
}

// PadSeq returns sequence padded for key (6 width, lexicographic sort)
func PadSeq(seq uint64) string {
	return fmt.Sprintf("%0*d", SeqPadWidth, seq)
}
