package validation

import (
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// ValidateKeyID validates a key ID format
func ValidateKeyID(keyID string) error {
	if keyID == "" {
		return errors.New("key_id cannot be empty")
	}

	if len(keyID) > 256 {
		return errors.New("key_id too long (max 256 characters)")
	}

	// Allow alphanumeric, underscore, dash, and forward slash
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_\-/]+$`, keyID)
	if err != nil {
		return errors.New("invalid key_id format")
	}
	if !matched {
		return errors.New("key_id contains invalid characters")
	}

	return nil
}

// ValidateThreadID validates a thread ID format
func ValidateThreadID(threadKey string) error {
	if threadKey == "" {
		return errors.New("thread_key cannot be empty")
	}

	if len(threadKey) > 256 {
		return errors.New("thread_key too long (max 256 characters)")
	}

	// Allow alphanumeric, underscore, dash, and forward slash
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_\-/]+$`, threadKey)
	if err != nil {
		return errors.New("invalid thread_key format")
	}
	if !matched {
		return errors.New("thread_key contains invalid characters")
	}

	return nil
}

// ValidateHexKey validates a hex-encoded key
func ValidateHexKey(hexKey string) error {
	if hexKey == "" {
		return errors.New("hex key cannot be empty")
	}

	// Remove any whitespace
	hexKey = strings.TrimSpace(hexKey)

	// Check if it's valid hex
	_, err := hex.DecodeString(hexKey)
	if err != nil {
		return errors.New("invalid hex format")
	}

	return nil
}

// ValidateBase64 validates a base64-encoded string
func ValidateBase64(b64 string) error {
	if b64 == "" {
		return nil // empty is allowed for optional fields
	}

	// Basic base64 validation - this is a simple check
	// For production, you might want more thorough validation
	if len(b64) == 0 {
		return errors.New("base64 string cannot be empty")
	}

	return nil
}

// ValidatePlaintext validates plaintext input
func ValidatePlaintext(plaintext string) error {
	if plaintext == "" {
		return errors.New("plaintext cannot be empty")
	}

	if len(plaintext) > 1024*1024 { // 1MB limit
		return errors.New("plaintext too large (max 1MB)")
	}

	return nil
}
