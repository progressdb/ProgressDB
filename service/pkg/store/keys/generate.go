package keys

import (
	"fmt"
)

// general
func GenThreadPrvKey(threadTS string) string {
	return fmt.Sprintf(ThreadPrvKey, threadTS)
}

func GenMessagePrvKey(threadTS string, messageKey string) string {
	if parsed, err := ParseKey(threadTS); err == nil && parsed.Type == KeyTypeThread {
		threadTS = parsed.ThreadTS
	}
	return fmt.Sprintf(MessagePrvKey, threadTS, messageKey)
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

func GenMessageVersionKey(messageKey string, ts int64, versionSeq uint64) string {
	return fmt.Sprintf(VersionKey, messageKey, PadTS(ts), PadSeq(versionSeq))
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
