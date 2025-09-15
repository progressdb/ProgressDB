package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// AuditSign signs audit bytes and returns base64 signature. Default: HMAC-SHA256
func AuditSign(b []byte) (string, error) {
	if len(key) == 0 {
		return "", nil
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// LockMemory/UnlockMemory provided by platform-specific files in `pkg/conn`.

// Exported helper used by callers outside this package.
func SecurityRandRead(b []byte) (int, error) { return securityRandReadImpl(b) }

// EncryptWithKeyBytes provides a helper to wrap dek with new raw key bytes.
func EncryptWithKeyBytes(kb, dek []byte) ([]byte, error) {
	if len(kb) != 32 {
		return nil, errors.New("invalid key length")
	}
	block, err := aes.NewCipher(kb)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, dek, nil)
	return append(nonce, ct...), nil
}

// EncryptWithRawKey encrypts plaintext using the provided raw key (DEK)
func EncryptWithRawKey(dek, plaintext []byte) ([]byte, error) {
	if len(dek) != 32 {
		return nil, errors.New("invalid dek length")
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, ct...), nil
}

// DecryptWithRawKey decrypts ciphertext produced by EncryptWithRawKey
func DecryptWithRawKey(dek, data []byte) ([]byte, error) {
	if len(dek) != 32 {
		return nil, errors.New("invalid dek length")
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce := data[:ns]
	ct := data[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}
