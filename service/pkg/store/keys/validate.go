package keys

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// conservative ID validation: letters, digits, dot, underscore, dash
	// and a reasonable upper bound to protect DB key shapes.
	idRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)
	// For keys, allow strict format matching
	messageKeyRegexp           = regexp.MustCompile(`^t:([A-Za-z0-9._-]{1,256}):m:([A-Za-z0-9._-]{1,256}):([0-9]{1,6})$`)
	messagePrvKeyRegexp        = regexp.MustCompile(`^t:([A-Za-z0-9._-]{1,256}):m:([A-Za-z0-9._-]{1,256})$`) // Matches MessagePrvKey = "t:%s:m:%s"
	versionKeyRegexp           = regexp.MustCompile(`^v:([A-Za-z0-9._-]{1,256}):([0-9]{1,20}):([0-9]{1,6})$`)
	threadKeyRegexp            = regexp.MustCompile(`^t:([A-Za-z0-9._-]{1,256})$`)
	threadPrvKeyRegexp         = regexp.MustCompile(`^t:([A-Za-z0-9._-]{1,256})$`)
	threadMessageStartRegexp   = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:start$`)
	threadMessageEndRegexp     = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:end$`)
	threadMessageCDeltasRegexp = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:cdeltas$`)
	threadMessageUDeltasRegexp = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:udeltas$`)
	threadMessageSkipsRegexp   = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:skips$`)
	threadMessageLCRegexp      = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:lc$`)
	threadMessageLURegexp      = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:lu$`)

	threadVersionStartRegexp   = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:start$`)
	threadVersionEndRegexp     = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:end$`)
	threadVersionCDeltasRegexp = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:cdeltas$`)
	threadVersionUDeltasRegexp = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:udeltas$`)
	threadVersionSkipsRegexp   = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:skips$`)
	threadVersionLCRegexp      = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:lc$`)
	threadVersionLURegexp      = regexp.MustCompile(`^idx:t:([A-Za-z0-9._-]{1,256}):ms:([A-Za-z0-9._-]{1,256}):vs:lu$`)

	softDeleteMarkerRegexp  = regexp.MustCompile(`^del:(.+)$`)
	relUserOwnsThreadRegexp = regexp.MustCompile(`^rel:u:([A-Za-z0-9._-]{1,256}):t:([A-Za-z0-9._-]{1,256})$`)
	relThreadHasUserRegexp  = regexp.MustCompile(`^rel:t:([A-Za-z0-9._-]{1,256}):u:([A-Za-z0-9._-]{1,256})$`)
)

func ValidateMessagePrvKey(key string) error {
	if !messagePrvKeyRegexp.MatchString(key) {
		return fmt.Errorf("invalid version key format: %q", key)
	}
	return nil
}

func ValidateUserID(id string) error {
	if id == "" {
		return errors.New("user id empty")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("invalid user id: %q", id)
	}
	return nil
}

// --- Key format validators ---

func ValidateMessageKey(key string) error {
	if !messageKeyRegexp.MatchString(key) {
		return fmt.Errorf("invalid message key format: %q", key)
	}
	return nil
}

func ValidateVersionKey(key string) error {
	if !versionKeyRegexp.MatchString(key) {
		return fmt.Errorf("invalid version key format: %q", key)
	}
	return nil
}

func ValidateThreadKey(key string) error {
	if !threadKeyRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread key format: %q", key)
	}
	return nil
}

func ValidateThreadPrvKey(key string) error {
	if !threadPrvKeyRegexp.MatchString(key) {
		return fmt.Errorf("invalid provisional thread key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageStart(key string) error {
	if !threadMessageStartRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message start key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageEnd(key string) error {
	if !threadMessageEndRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message end key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageCDeltas(key string) error {
	if !threadMessageCDeltasRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message cdeltas key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageUDeltas(key string) error {
	if !threadMessageUDeltasRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message udeltas key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageSkips(key string) error {
	if !threadMessageSkipsRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message skips key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageLC(key string) error {
	if !threadMessageLCRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message lc key format: %q", key)
	}
	return nil
}

func ValidateThreadMessageLU(key string) error {
	if !threadMessageLURegexp.MatchString(key) {
		return fmt.Errorf("invalid thread message lu key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionStart(key string) error {
	if !threadVersionStartRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version start key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionEnd(key string) error {
	if !threadVersionEndRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version end key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionCDeltas(key string) error {
	if !threadVersionCDeltasRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version cdeltas key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionUDeltas(key string) error {
	if !threadVersionUDeltasRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version udeltas key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionSkips(key string) error {
	if !threadVersionSkipsRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version skips key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionLC(key string) error {
	if !threadVersionLCRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version lc key format: %q", key)
	}
	return nil
}

func ValidateThreadVersionLU(key string) error {
	if !threadVersionLURegexp.MatchString(key) {
		return fmt.Errorf("invalid thread version lu key format: %q", key)
	}
	return nil
}

func ValidateSoftDeleteMarkerKey(key string) error {
	if !softDeleteMarkerRegexp.MatchString(key) {
		return fmt.Errorf("invalid soft delete marker key format: %q", key)
	}
	return nil
}

func ValidateUserOwnsThreadKey(key string) error {
	if !relUserOwnsThreadRegexp.MatchString(key) {
		return fmt.Errorf("invalid user owns thread key format: %q", key)
	}
	return nil
}

func ValidateThreadHasUserKey(key string) error {
	if !relThreadHasUserRegexp.MatchString(key) {
		return fmt.Errorf("invalid thread has user key format: %q", key)
	}
	return nil
}

// --- Key validation functions ---

func ValidateKey(key string) error {
	if key == "" {
		return errors.New("key empty")
	}
	if !idRegexp.MatchString(key) {
		return fmt.Errorf("invalid key: %q", key)
	}
	return nil
}

// IsProvisionalMessageKey checks if a message key is in provisional format
// Returns true if it's a simple message key or a structured provisional key (not a full sequenced key)
func IsProvisionalMessageKey(messageKey string) bool {
	count := strings.Count(messageKey, ":")
	// Structured provisional key: e.g., "t:threadID:m:msgID" (3 colons)
	if count == 3 && strings.Contains(messageKey, "t:") && strings.Contains(messageKey, ":m:") {
		return true
	}
	// Simple key: e.g., "msgID" (0 colons)
	if count == 0 && ValidateKey(messageKey) == nil {
		return true
	}
	return false
}
