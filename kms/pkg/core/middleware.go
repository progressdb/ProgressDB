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

func AuditSign(b []byte) (string, error) {
	if len(key) == 0 {
		return "", nil
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func SecurityRandRead(b []byte) (int, error) { return securityRandReadImpl(b) }

func WrapDEKWithKeyBytes(kb, dek []byte) ([]byte, error) {
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
