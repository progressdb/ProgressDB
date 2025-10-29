package keys

import (
	"fmt"
)

// general
func GenThreadPrvKey(threadTS string) string {
	return fmt.Sprintf(ThreadPrvKey, threadTS)
}

func GenMessagePrvKey(threadTS string, messageID string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(MessagePrvKey, threadTS, messageID)
}

func GenMessageKey(threadTS, messageTS string, seq uint64) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	if parts, err := ParseVersionKey(messageTS); err == nil {
		messageTS = parts.MessageKey
	}
	return fmt.Sprintf(MessageKey, threadTS, messageTS, PadSeq(seq))
}

func GenVersionKey(messageTS string, ts int64, seq uint64) string {
	return fmt.Sprintf(VersionKey, messageTS, PadTS(ts), PadSeq(seq))
}

func GenThreadKey(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadKey, threadTS)
}

// threading
func GenThreadMessageStart(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageStart, threadTS)
}
func GenThreadMessageEnd(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageEnd, threadTS)
}
func GenThreadMessageCDeltas(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageCDeltas, threadTS)
}
func GenThreadMessageUDeltas(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageUDeltas, threadTS)
}
func GenThreadMessageSkips(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageSkips, threadTS)
}
func GenThreadMessageLC(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageLC, threadTS)
}
func GenThreadMessageLU(threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadMessageLU, threadTS)
}

// versioning
func GenThreadVersionStart(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionStart, threadTS, messageTS)
}
func GenThreadVersionEnd(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionEnd, threadTS, messageTS)
}
func GenThreadVersionCDeltas(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionCDeltas, threadTS, messageTS)
}
func GenThreadVersionUDeltas(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionUDeltas, threadTS, messageTS)
}
func GenThreadVersionSkips(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionSkips, threadTS, messageTS)
}
func GenThreadVersionLC(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionLC, threadTS, messageTS)
}
func GenThreadVersionLU(threadTS, messageTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(ThreadVersionLU, threadTS, messageTS)
}

// deletes
func GenSoftDeleteMarkerKey(originalKey string) string {
	return fmt.Sprintf(SoftDeleteMarker, originalKey)
}

// relationships
func GenUserOwnsThreadKey(userID, threadTS string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(RelUserOwnsThread, userID, threadTS)
}

func GenThreadHasUserKey(threadTS, userID string) string {
	if parts, err := ParseThreadKey(threadTS); err == nil {
		threadTS = parts.ThreadKey
	}
	return fmt.Sprintf(RelThreadHasUser, threadTS, userID)
}

// helpers
func PadTS(ts int64) string {
	return fmt.Sprintf("%0*d", TSPadWidth, ts)
}

func PadSeq(seq uint64) string {
	return fmt.Sprintf("%0*d", SeqPadWidth, seq)
}
