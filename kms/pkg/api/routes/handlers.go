package handlers

import (
	"encoding/base64"

	"github.com/progressdb/kms/pkg/store"
)

type ProviderInterface interface {
	CreateDEKForThread(threadID string) (string, []byte, string, string, error)
	EncryptWithDEK(dekID string, plaintext, aad []byte) ([]byte, string, error)
	DecryptWithDEK(dekID string, ciphertext, aad []byte) ([]byte, error)
	RewrapDEKForThread(dekID string, newKEKHex string) ([]byte, string, string, error)
	Enabled() bool
	Health() error
	Close() error
}

type Dependencies struct {
	Provider ProviderInterface
	Store    *store.Store
}

func mustDecodeBase64(s string) []byte {
	if s == "" {
		return nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b
	}
	return []byte(s)
}
