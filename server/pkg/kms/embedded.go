package kms

import (
	"context"

	kmss "github.com/progressdb/kms/pkg/security"
)

// hkAdapter adapts the kms/pkg/security.KMSProvider to the server's
// KMSProvider interface expected by the rest of the code.
type hkAdapter struct {
	p kmss.KMSProvider
}

func (h *hkAdapter) Enabled() bool { return h.p.Enabled() }
func (h *hkAdapter) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	return h.p.Encrypt(plaintext, aad)
}
func (h *hkAdapter) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	return h.p.Decrypt(ciphertext, iv, aad)
}
func (h *hkAdapter) CreateDEK() (string, []byte, string, string, error) {
	kid, wrapped, err := h.p.CreateDEK()
	if err != nil {
		return "", nil, "", "", err
	}
	// underlying provider doesn't provide kek metadata; return empty strings
	return kid, wrapped, "", "", nil
}
func (h *hkAdapter) CreateDEKForThread(threadID string) (string, []byte, string, string, error) {
	kid, wrapped, err := h.p.CreateDEKForThread(threadID)
	if err != nil {
		return "", nil, "", "", err
	}
	return kid, wrapped, "", "", nil
}
func (h *hkAdapter) EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	return h.p.EncryptWithKey(keyID, plaintext, aad)
}
func (h *hkAdapter) DecryptWithKey(keyID string, ciphertext, iv, aad []byte) ([]byte, error) {
	return h.p.DecryptWithKey(keyID, ciphertext, iv, aad)
}
func (h *hkAdapter) WrapDEK(dek []byte) ([]byte, error)       { return h.p.WrapDEK(dek) }
func (h *hkAdapter) UnwrapDEK(wrapped []byte) ([]byte, error) { return h.p.UnwrapDEK(wrapped) }
func (h *hkAdapter) GetWrapped(keyID string) ([]byte, error)  { return h.p.GetWrapped(keyID) }
func (h *hkAdapter) Health() error                            { return h.p.Health() }
func (h *hkAdapter) Close() error                             { return h.p.Close() }

// NewHashicorpEmbeddedProvider constructs a HashiCorp AEAD-backed provider
// from the provided master key hex string and returns an adapter that
// implements the server's `kms.KMSProvider` interface.
func NewHashicorpEmbeddedProvider(ctx context.Context, masterHex string) (KMSProvider, error) {
	p, err := kmss.NewHashicorpProviderFromHex(ctx, masterHex)
	if err != nil {
		return nil, err
	}
	return &hkAdapter{p: p}, nil
}
