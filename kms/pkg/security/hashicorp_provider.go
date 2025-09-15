package security

import (
	"context"
    "encoding/base64"
    "encoding/hex"
    "errors"
    "fmt"
    "time"

	wrapping "github.com/hashicorp/go-kms-wrapping/v2"
	aead "github.com/hashicorp/go-kms-wrapping/v2/aead"
)

type hashicorpProvider struct {
	ctx context.Context
	w   *aead.Wrapper
}

// NewHashicorpProvider creates a provider backed by the aead wrapper using a raw 32-byte key (not hex).
func NewHashicorpProviderFromRaw(ctx context.Context, key []byte) (KMSProvider, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid key length")
	}
	w := aead.NewWrapper()
	// configure with raw key via WithConfigMap
	cfg := map[string]string{"key": base64.StdEncoding.EncodeToString(key), "key_id": "local"}
	if _, err := w.SetConfig(ctx, wrapping.WithConfigMap(cfg)); err != nil {
		return nil, fmt.Errorf("wrapper setconfig failed: %w", err)
	}
	return &hashicorpProvider{ctx: ctx, w: w}, nil
}

func (h *hashicorpProvider) Enabled() bool {
	return h != nil && h.w != nil
}

func (h *hashicorpProvider) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	info, err := h.w.Encrypt(h.ctx, plaintext, wrapping.WithAad(aad))
	if err != nil {
		return nil, nil, "", err
	}
	if info == nil || len(info.Ciphertext) == 0 {
		return nil, nil, "", errors.New("encrypt returned empty")
	}
	iv = info.Ciphertext[:12]
	ct := info.Ciphertext[12:]
	// try to get key id
	keyId, _ := h.w.KeyId(h.ctx)
	return ct, iv, keyId, nil
}

func (h *hashicorpProvider) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
    keyId, _ := h.w.KeyId(h.ctx)
    info := &wrapping.BlobInfo{Ciphertext: append(iv, ciphertext...), KeyInfo: &wrapping.KeyInfo{KeyId: keyId}}
    return h.w.Decrypt(h.ctx, info, wrapping.WithAad(aad))
}

func (h *hashicorpProvider) CreateDEK() (string, []byte, error) {
	dek := make([]byte, 32)
	if _, err := SecurityRandRead(dek); err != nil {
		return "", nil, err
	}
	info, err := h.w.Encrypt(h.ctx, dek)
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		return "", nil, err
	}
    return fmt.Sprintf("k_%d", time.Now().UnixNano()), info.Ciphertext, nil
}

func (h *hashicorpProvider) CreateDEKForThread(threadID string) (string, []byte, error) {
	return h.CreateDEK()
}

func (h *hashicorpProvider) EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	return h.Encrypt(plaintext, aad)
}

func (h *hashicorpProvider) DecryptWithKey(keyID string, ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	return h.Decrypt(ciphertext, iv, aad)
}

func (h *hashicorpProvider) WrapDEK(dek []byte) ([]byte, error) {
	info, err := h.w.Encrypt(h.ctx, dek)
	if err != nil {
		return nil, err
	}
	return info.Ciphertext, nil
}

func (h *hashicorpProvider) UnwrapDEK(wrapped []byte) ([]byte, error) {
	info := &wrapping.BlobInfo{Ciphertext: wrapped}
	return h.w.Decrypt(h.ctx, info)
}

func (h *hashicorpProvider) GetWrapped(keyID string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (h *hashicorpProvider) Health() error { return nil }

func (h *hashicorpProvider) Close() error {
    if f, ok := any(h.w).(interface{ Finalize(context.Context, ...wrapping.Option) error }); ok {
        return f.Finalize(h.ctx)
    }
    return nil
}

// helper to create provider from hex key string
func NewHashicorpProviderFromHex(ctx context.Context, hexKey string) (KMSProvider, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, errors.New("hex key must be 32 bytes")
	}
	return NewHashicorpProviderFromRaw(ctx, b)
}
