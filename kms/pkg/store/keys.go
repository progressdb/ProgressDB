package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	DEKPrefix = "dek:"
)

func GenerateDEKKey() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("dek_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func FormatDEKKey(keyID string) string {
	return DEKPrefix + keyID
}

func ParseDEKKey(storedKey string) (string, error) {
	if !strings.HasPrefix(storedKey, DEKPrefix) {
		return "", fmt.Errorf("invalid DEK key format: missing prefix %s", DEKPrefix)
	}
	return strings.TrimPrefix(storedKey, DEKPrefix), nil
}

func IsDEKKey(key string) bool {
	return strings.HasPrefix(key, DEKPrefix)
}
