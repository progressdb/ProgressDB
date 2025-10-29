package keys

import (
	"fmt"
)

// general
func GenThreadPrvKey(threadTS string) string {
	return fmt.Sprintf(ThreadPrvKey, threadTS)
}

func GenMessagePrvKey(threadTS string, messageID string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(MessagePrvKey, threadTS, messageID)
}

func GenMessageKey(threadTS, messageTS string, seq uint64) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	if parsed, err := ParseKey(messageTS); err == nil && parsed.Type == KeyTypeVersion {
		messageTS = parsed.MessageTS
	}
	return fmt.Sprintf(MessageKey, threadTS, messageTS, PadSeq(seq))
}

func GenVersionKey(messageTS string, ts int64, seq uint64) string {
	return fmt.Sprintf(VersionKey, messageTS, PadTS(ts), PadSeq(seq))
}

func GenThreadKey(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadKey, threadTS)
}

// threading
func GenThreadMessageStart(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageStart, threadTS)
}
func GenThreadMessageEnd(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageEnd, threadTS)
}
func GenThreadMessageCDeltas(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageCDeltas, threadTS)
}
func GenThreadMessageUDeltas(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageUDeltas, threadTS)
}
func GenThreadMessageSkips(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageSkips, threadTS)
}
func GenThreadMessageLC(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageLC, threadTS)
}
func GenThreadMessageLU(threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadMessageLU, threadTS)
}

// versioning
func GenThreadVersionStart(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionStart, threadTS, messageTS)
}
func GenThreadVersionEnd(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionEnd, threadTS, messageTS)
}
func GenThreadVersionCDeltas(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionCDeltas, threadTS, messageTS)
}
func GenThreadVersionUDeltas(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionUDeltas, threadTS, messageTS)
}
func GenThreadVersionSkips(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionSkips, threadTS, messageTS)
}
func GenThreadVersionLC(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionLC, threadTS, messageTS)
}
func GenThreadVersionLU(threadTS, messageTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(ThreadVersionLU, threadTS, messageTS)
}

// deletes
func GenSoftDeleteMarkerKey(originalKey string) string {
	return fmt.Sprintf(SoftDeleteMarker, originalKey)
}

// relationships
func GenUserOwnsThreadKey(userID, threadTS string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(RelUserOwnsThread, userID, threadTS)
}

func GenThreadHasUserKey(threadTS, userID string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
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
