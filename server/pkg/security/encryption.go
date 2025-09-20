package security

import (
    "encoding/hex"
    "errors"

    "progressdb/pkg/kms"
)

var key []byte
var keyLocked bool

// SetKeyHex sets the AES-256 master key (hex string). An empty string clears it.
func SetKeyHex(hexKey string) error {
    if hexKey == "" {
        if key != nil && keyLocked {
            _ = UnlockMemory(key)
            keyLocked = false
        }
        key = nil
        return nil
    }
    b, err := hex.DecodeString(hexKey)
    if err != nil {
        return err
    }
    if l := len(b); l != 32 {
        return errors.New("encryption key must be 32 bytes (AES-256)")
    }
    if key != nil && keyLocked {
        _ = UnlockMemory(key)
        keyLocked = false
    }
    key = b
    if err := LockMemory(key); err == nil {
        keyLocked = true
    }
    return nil
}

// EncryptionEnabled reports whether encryption is available (provider or local key).
func EncryptionEnabled() bool {
    if kms.IsProviderEnabled() {
        return true
    }
    return key != nil && len(key) == 32
}

func EncryptionHasFieldPolicy() bool { return false }