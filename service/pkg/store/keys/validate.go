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

// ValidationResult contains the result of key validation
type ValidationResult struct {
	Type   KeyType
	Valid  bool
	Error  error
	Parsed *KeyParts // Optional parsed parts if validation succeeds
}

// parsedToKeyParts converts various parsed types to *KeyParts for unified interface
func parsedToKeyParts(parsed interface{}) *KeyParts {
	switch p := parsed.(type) {
	case *KeyParts:
		return p
	case *ThreadMetaParts:
		return &KeyParts{
			Type:      KeyTypeThread,
			ThreadKey: p.ThreadKey,
		}
	case *MessageKeyParts:
		// Parse the key to get full KeyParts
		fullKey, _ := ParseKey(p.ThreadKey) // Use threadKey to get base
		if fullKey != nil {
			fullKey.Type = KeyTypeMessage
			fullKey.MessageKey = p.MessageKey
			fullKey.Seq = p.Seq
		}
		return fullKey
	case *VersionKeyParts:
		// Parse the version key to get full KeyParts
		fullKey, _ := ParseKey("v:" + p.MessageKey + ":" + p.MessageTS + ":placeholder")
		if fullKey != nil {
			fullKey.Type = KeyTypeVersion
			fullKey.MessageTS = p.MessageKey
			fullKey.VersionTS = p.MessageTS
			fullKey.Seq = p.Seq
		}
		return fullKey
	case *UserOwnsThreadParts:
		return &KeyParts{
			Type:      KeyTypeUserOwnsThread,
			UserID:    p.UserID,
			ThreadKey: p.ThreadKey,
		}
	case *ThreadMessageStartParts:
		return &KeyParts{
			Type:      KeyTypeThreadMessageStart,
			ThreadKey: p.ThreadKey,
		}
	case *ThreadMessageEndParts:
		return &KeyParts{
			Type:      KeyTypeThreadMessageEnd,
			ThreadKey: p.ThreadKey,
		}
	case *ThreadMessageLCIndexParts:
		return &KeyParts{
			Type:      KeyTypeThreadMessageLC,
			ThreadKey: p.ThreadKey,
		}
	case *ThreadMessageLUIndexParts:
		return &KeyParts{
			Type:      KeyTypeThreadMessageLU,
			ThreadKey: p.ThreadKey,
		}
	case *ThreadVersionStartParts:
		return &KeyParts{
			Type:      KeyTypeThreadVersionStart,
			ThreadKey: p.ThreadKey,
			MessageTS: p.MessageKey,
		}
	case *ThreadVersionEndParts:
		return &KeyParts{
			Type:      KeyTypeThreadVersionEnd,
			ThreadKey: p.ThreadKey,
			MessageTS: p.MessageKey,
		}
	case *ThreadVersionLCIndexParts:
		return &KeyParts{
			Type:      KeyTypeThreadVersionLC,
			ThreadKey: p.ThreadKey,
			MessageTS: p.MessageKey,
		}
	case *ThreadVersionLUIndexParts:
		return &KeyParts{
			Type:      KeyTypeThreadVersionLU,
			ThreadKey: p.ThreadKey,
			MessageTS: p.MessageKey,
		}
	default:
		return nil
	}
}

// ValidateKey provides a unified interface for validating all types of keys
// It automatically detects the key type and returns validation result with type information
func ValidateKey(key string) *ValidationResult {
	if key == "" {
		return &ValidationResult{
			Type:  "",
			Valid: false,
			Error: errors.New("key cannot be empty"),
		}
	}

	// Try to parse key to determine its type
	parts := strings.Split(key, ":")
	if len(parts) < 2 {
		return &ValidationResult{
			Type:  "",
			Valid: false,
			Error: fmt.Errorf("invalid key format: %s", key),
		}
	}

	switch parts[0] {
	case "t":
		return validateThreadBasedKey(key, parts)
	case "v":
		return validateVersionKeyFormat(key, parts)
	case "rel":
		return validateRelationKeyFormat(key, parts)
	case "idx":
		return validateIndexKeyFormat(key, parts)
	case "sd":
		return validateSoftDeleteMarker(key)
	default:
		// For simple keys without prefixes, use basic validation
		if !idRegexp.MatchString(key) {
			return &ValidationResult{
				Type:  "",
				Valid: false,
				Error: fmt.Errorf("invalid key: %q", key),
			}
		}
		return &ValidationResult{
			Type:   "simple",
			Valid:  true,
			Parsed: nil,
		}
	}
}

// validateThreadBasedKey handles validation for keys starting with "t:"
func validateThreadBasedKey(key string, parts []string) *ValidationResult {
	if len(parts) < 2 {
		return &ValidationResult{
			Type:  "",
			Valid: false,
			Error: fmt.Errorf("invalid thread-based key format: %s", key),
		}
	}

	// t:{threadTS} - thread metadata
	if len(parts) == 2 {
		err := ValidateThreadKey(key)
		if err != nil {
			return &ValidationResult{
				Type:  KeyTypeThread,
				Valid: false,
				Error: err,
			}
		}
		parsed, _ := ParseThreadKey(key)
		return &ValidationResult{
			Type:   KeyTypeThread,
			Valid:  true,
			Parsed: parsedToKeyParts(parsed),
		}
	}

	// t:{threadTS}:m:{messageTS}[:{seq}] - message keys
	if len(parts) >= 4 && parts[2] == "m" {
		if len(parts) == 4 {
			err := ValidateMessagePrvKey(key)
			if err != nil {
				return &ValidationResult{
					Type:  KeyTypeMessageProvisional,
					Valid: false,
					Error: err,
				}
			}
			parsed, _ := ParseMessageProvisionalKey(key)
			return &ValidationResult{
				Type:   KeyTypeMessageProvisional,
				Valid:  true,
				Parsed: parsedToKeyParts(parsed),
			}
		} else if len(parts) == 5 {
			err := ValidateMessageKey(key)
			if err != nil {
				return &ValidationResult{
					Type:  KeyTypeMessage,
					Valid: false,
					Error: err,
				}
			}
			parsed, _ := ParseMessageKey(key)
			return &ValidationResult{
				Type:   KeyTypeMessage,
				Valid:  true,
				Parsed: parsedToKeyParts(parsed),
			}
		}
	}

	return &ValidationResult{
		Type:  "",
		Valid: false,
		Error: fmt.Errorf("invalid thread-based key format: %s", key),
	}
}

// validateVersionKeyFormat handles validation for keys starting with "v:"
func validateVersionKeyFormat(key string, parts []string) *ValidationResult {
	if len(parts) != 4 {
		return &ValidationResult{
			Type:  "",
			Valid: false,
			Error: fmt.Errorf("invalid version key format: %s", key),
		}
	}
	err := ValidateVersionKey(key)
	if err != nil {
		return &ValidationResult{
			Type:  KeyTypeVersion,
			Valid: false,
			Error: err,
		}
	}
	parsed, _ := ParseVersionKey(key)
	return &ValidationResult{
		Type:   KeyTypeVersion,
		Valid:  true,
		Parsed: parsedToKeyParts(parsed),
	}
}

// validateRelationKeyFormat handles validation for keys starting with "rel:"
func validateRelationKeyFormat(key string, parts []string) *ValidationResult {
	// rel:u:{userID}:t:{threadTS}
	if len(parts) == 5 && parts[1] == "u" && parts[3] == "t" {
		err := ValidateUserOwnsThreadKey(key)
		if err != nil {
			return &ValidationResult{
				Type:  KeyTypeUserOwnsThread,
				Valid: false,
				Error: err,
			}
		}
		parsed, _ := ParseUserOwnsThread(key)
		return &ValidationResult{
			Type:   KeyTypeUserOwnsThread,
			Valid:  true,
			Parsed: parsedToKeyParts(parsed),
		}
	}

	// rel:t:{threadTS}:u:{userID}
	if len(parts) == 5 && parts[1] == "t" && parts[3] == "u" {
		err := ValidateThreadHasUserKey(key)
		if err != nil {
			return &ValidationResult{
				Type:  "",
				Valid: false,
				Error: err,
			}
		}
		return &ValidationResult{
			Type:   "thread_has_user",
			Valid:  true,
			Parsed: nil,
		}
	}

	return &ValidationResult{
		Type:  "",
		Valid: false,
		Error: fmt.Errorf("invalid relation key format: %s", key),
	}
}

// validateIndexKeyFormat handles validation for keys starting with "idx:"
func validateIndexKeyFormat(key string, parts []string) *ValidationResult {
	if len(parts) < 5 {
		return &ValidationResult{
			Type:  "",
			Valid: false,
			Error: fmt.Errorf("invalid index key format: %s", key),
		}
	}

	// idx:t:{threadTS}:ms:{type}
	if parts[1] == "t" && parts[3] == "ms" {
		var keyType KeyType
		var err error
		var parsed interface{}

		switch parts[4] {
		case "start":
			keyType = KeyTypeThreadMessageStart
			err = ValidateThreadMessageStart(key)
			if err == nil {
				parsed, _ = ParseThreadMessageStart(key)
			}
		case "end":
			keyType = KeyTypeThreadMessageEnd
			err = ValidateThreadMessageEnd(key)
			if err == nil {
				parsed, _ = ParseThreadMessageEnd(key)
			}
		case "lc":
			keyType = KeyTypeThreadMessageLC
			err = ValidateThreadMessageLC(key)
			if err == nil {
				parsed, _ = ParseThreadMessageLC(key)
			}
		case "lu":
			keyType = KeyTypeThreadMessageLU
			err = ValidateThreadMessageLU(key)
			if err == nil {
				parsed, _ = ParseThreadMessageLU(key)
			}
		case "cdeltas":
			keyType = KeyTypeThreadMessageCDeltas
			err = ValidateThreadMessageCDeltas(key)
			if err == nil {
				parsed, _ = ParseThreadMessageCDeltas(key)
			}
		case "udeltas":
			keyType = KeyTypeThreadMessageUDeltas
			err = ValidateThreadMessageUDeltas(key)
			if err == nil {
				parsed, _ = ParseThreadMessageUDeltas(key)
			}
		case "skips":
			keyType = KeyTypeThreadMessageSkips
			err = ValidateThreadMessageSkips(key)
			if err == nil {
				parsed, _ = ParseThreadMessageSkips(key)
			}
		default:
			return &ValidationResult{
				Type:  "",
				Valid: false,
				Error: fmt.Errorf("unknown thread message index type: %s", parts[4]),
			}
		}

		if err != nil {
			return &ValidationResult{
				Type:  keyType,
				Valid: false,
				Error: err,
			}
		}
		return &ValidationResult{
			Type:   keyType,
			Valid:  true,
			Parsed: parsedToKeyParts(parsed),
		}
	}

	// idx:t:{threadTS}:ms:{messageTS}:v:{type}
	if len(parts) >= 7 && parts[1] == "t" && parts[3] == "ms" && parts[5] == "v" {
		var keyType KeyType
		var err error
		var parsed interface{}

		switch parts[6] {
		case "start":
			keyType = KeyTypeThreadVersionStart
			err = ValidateThreadVersionStart(key)
			if err == nil {
				parsed, _ = ParseThreadVersionStart(key)
			}
		case "end":
			keyType = KeyTypeThreadVersionEnd
			err = ValidateThreadVersionEnd(key)
			if err == nil {
				parsed, _ = ParseThreadVersionEnd(key)
			}
		case "lc":
			keyType = KeyTypeThreadVersionLC
			err = ValidateThreadVersionLC(key)
			if err == nil {
				parsed, _ = ParseThreadVersionLC(key)
			}
		case "lu":
			keyType = KeyTypeThreadVersionLU
			err = ValidateThreadVersionLU(key)
			if err == nil {
				parsed, _ = ParseThreadVersionLU(key)
			}
		case "cdeltas":
			keyType = KeyTypeThreadVersionCDeltas
			err = ValidateThreadVersionCDeltas(key)
			if err == nil {
				parsed, _ = ParseThreadVersionCDeltas(key)
			}
		case "udeltas":
			keyType = KeyTypeThreadVersionUDeltas
			err = ValidateThreadVersionUDeltas(key)
			if err == nil {
				parsed, _ = ParseThreadVersionUDeltas(key)
			}
		case "skips":
			keyType = KeyTypeThreadVersionSkips
			err = ValidateThreadVersionSkips(key)
			if err == nil {
				parsed, _ = ParseThreadVersionSkips(key)
			}
		default:
			return &ValidationResult{
				Type:  "",
				Valid: false,
				Error: fmt.Errorf("unknown thread version index type: %s", parts[6]),
			}
		}

		if err != nil {
			return &ValidationResult{
				Type:  keyType,
				Valid: false,
				Error: err,
			}
		}
		return &ValidationResult{
			Type:   keyType,
			Valid:  true,
			Parsed: parsedToKeyParts(parsed),
		}
	}

	// idx:t:deleted:u:{userID}:list
	if len(parts) == 6 && parts[1] == "t" && parts[2] == "deleted" && parts[3] == "u" && parts[5] == "list" {
		_, err := ParseDeletedThreadsIndex(key)
		if err != nil {
			return &ValidationResult{
				Type:  KeyTypeDeletedThreadsIndex,
				Valid: false,
				Error: err,
			}
		}
		return &ValidationResult{
			Type:   KeyTypeDeletedThreadsIndex,
			Valid:  true,
			Parsed: nil, // No specific parser return type
		}
	}

	// idx:m:deleted:u:{userID}:list
	if len(parts) == 6 && parts[1] == "m" && parts[2] == "deleted" && parts[3] == "u" && parts[5] == "list" {
		_, err := ParseDeletedMessagesIndex(key)
		if err != nil {
			return &ValidationResult{
				Type:  KeyTypeDeletedMessagesIndex,
				Valid: false,
				Error: err,
			}
		}
		return &ValidationResult{
			Type:   KeyTypeDeletedMessagesIndex,
			Valid:  true,
			Parsed: nil, // No specific parser return type
		}
	}

	return &ValidationResult{
		Type:  "",
		Valid: false,
		Error: fmt.Errorf("invalid index key format: %s", key),
	}
}

// validateSoftDeleteMarker handles validation for keys starting with "sd:"
func validateSoftDeleteMarker(key string) *ValidationResult {
	err := ValidateSoftDeleteMarkerKey(key)
	if err != nil {
		return &ValidationResult{
			Type:  "soft_delete_marker",
			Valid: false,
			Error: err,
		}
	}
	return &ValidationResult{
		Type:   "soft_delete_marker",
		Valid:  true,
		Parsed: nil,
	}
}

// IsProvisionalMessageKey checks if a message key is in provisional format
// Returns true if it's a simple message key or a structured provisional key (not a full sequenced key)
func IsProvisionalMessageKey(messageKey string) bool {
	count := strings.Count(messageKey, ":")
	// Structured provisional key: e.g., "t:threadKey:m:messageKey" (3 colons)
	if count == 3 && strings.Contains(messageKey, "t:") && strings.Contains(messageKey, ":m:") {
		return true
	}
	// Simple key: e.g., "messageKey" (0 colons)
	if count == 0 {
		result := ValidateKey(messageKey)
		return result.Valid && result.Type == "simple"
	}
	return false
}
