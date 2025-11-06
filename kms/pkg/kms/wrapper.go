package kms

import (
	"context"
	"encoding/base64"
	"fmt"

	wrapping "github.com/hashicorp/go-kms-wrapping/v2"
	"github.com/hashicorp/go-kms-wrapping/v2/aead"
)

type Wrapper interface {
	Wrap(ctx context.Context, plaintext []byte) (*wrapping.BlobInfo, error)
	Unwrap(ctx context.Context, blob *wrapping.BlobInfo) ([]byte, error)
	KeyInfo() (keyID string, keyVersion string)
	Close() error
}

type wrapper struct {
	wrapper wrapping.Wrapper
}

func NewWrapper(ctx context.Context, masterKey []byte) (Wrapper, error) {
	aeadWrapper := aead.NewWrapper()

	// HashiCorp AEAD wrapper expects base64-encoded key
	keyBase64 := base64.StdEncoding.EncodeToString(masterKey)

	_, err := aeadWrapper.SetConfig(ctx, wrapping.WithConfigMap(map[string]string{
		"key": keyBase64,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to configure AEAD wrapper: %w", err)
	}

	return &wrapper{wrapper: aeadWrapper}, nil
}

func NewWrapperFromHex(ctx context.Context, masterKeyHex string) (Wrapper, error) {
	return NewWrapper(ctx, []byte(masterKeyHex))
}

func (w *wrapper) Wrap(ctx context.Context, plaintext []byte) (*wrapping.BlobInfo, error) {
	return w.wrapper.Encrypt(ctx, plaintext)
}

func (w *wrapper) Unwrap(ctx context.Context, blob *wrapping.BlobInfo) ([]byte, error) {
	return w.wrapper.Decrypt(ctx, blob)
}

func (w *wrapper) KeyInfo() (keyID string, keyVersion string) {
	if w.wrapper == nil {
		return "", ""
	}
	keyID, _ = w.wrapper.KeyId(context.Background())
	return keyID, ""
}

func (w *wrapper) Close() error {
	return nil
}
