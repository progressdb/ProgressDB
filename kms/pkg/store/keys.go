package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// DEKPrefix is the prefix for DEK keys in storage
	DEKPrefix = "dek:"
)

// GenerateDEKKey generates a unique DEK key ID
func GenerateDEKKey() string {
	// Generate 16 random bytes and encode as hex
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("dek_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// FormatDEKKey formats a key ID with the DEK prefix for storage
func FormatDEKKey(keyID string) string {
	return DEKPrefix + keyID
}

// ParseDEKKey extracts the key ID from a stored DEK key
func ParseDEKKey(storedKey string) (string, error) {
	if !strings.HasPrefix(storedKey, DEKPrefix) {
		return "", fmt.Errorf("invalid DEK key format: missing prefix %s", DEKPrefix)
	}
	return strings.TrimPrefix(storedKey, DEKPrefix), nil
}

// IsDEKKey checks if a key is a DEK key
func IsDEKKey(key string) bool {
	return strings.HasPrefix(key, DEKPrefix)
}
