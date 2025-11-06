package kms

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	wrapping "github.com/hashicorp/go-kms-wrapping/v2"
	"github.com/progressdb/kms/pkg/store"
)

type KMS struct {
	ctx     context.Context
	wrapper Wrapper
	store   *store.Store
	mu      sync.RWMutex
	deks    map[string]*secureBytes
}

type DEK struct {
	KeyID      string            `json:"key_id"`
	WrappedDEK json.RawMessage   `json:"wrapped_dek"`
	KekID      string            `json:"kek_id"`
	KekVersion string            `json:"kek_version"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

func New(ctx context.Context, store *store.Store, masterKey []byte) (*KMS, error) {
	wrapper, err := NewWrapper(ctx, masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create wrapper: %w", err)
	}

	k := &KMS{
		ctx:     ctx,
		wrapper: wrapper,
		store:   store,
		deks:    make(map[string]*secureBytes),
	}

	if store != nil {
		if err := k.loadDEKs(); err != nil {
			return nil, fmt.Errorf("failed to load DEKs: %w", err)
		}
	}

	return k, nil
}

func (k *KMS) CreateDEK(keyID ...string) (*DEK, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("failed to generate DEK: %w", err)
	}

	var finalKeyID string
	if len(keyID) > 0 && keyID[0] != "" {
		finalKeyID = keyID[0]
	} else {
		finalKeyID = store.GenerateDEKKey()
	}

	wrappedBlob, err := k.wrapper.Wrap(k.ctx, dek)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap DEK: %w", err)
	}

	secureWipe(dek)

	wrappedData, err := json.Marshal(wrappedBlob)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal wrapped DEK: %w", err)
	}

	if err := k.store.SaveKeyMeta(finalKeyID, wrappedData); err != nil {
		return nil, fmt.Errorf("failed to store DEK: %w", err)
	}

	k.mu.Lock()
	k.deks[finalKeyID] = newSecureBytes(wrappedData)
	k.mu.Unlock()

	kekID, kekVersion := k.wrapper.KeyInfo()

	return &DEK{
		KeyID:      finalKeyID,
		WrappedDEK: wrappedData,
		KekID:      kekID,
		KekVersion: kekVersion,
		CreatedAt:  time.Now(),
	}, nil
}

func (k *KMS) Encrypt(keyID string, plaintext []byte) ([]byte, error) {
	dek, err := k.getDEK(keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve DEK: %w", err)
	}
	defer secureWipe(dek)

	blobInfo, err := k.wrapper.Wrap(k.ctx, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt: %w", err)
	}

	return json.Marshal(blobInfo)
}

func (k *KMS) Decrypt(keyID string, ciphertext []byte) ([]byte, error) {
	var blobInfo wrapping.BlobInfo
	if err := json.Unmarshal(ciphertext, &blobInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ciphertext: %w", err)
	}

	plaintext, err := k.wrapper.Unwrap(k.ctx, &blobInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func (k *KMS) getDEK(keyID string) ([]byte, error) {
	k.mu.RLock()
	secureWrapped, ok := k.deks[keyID]
	k.mu.RUnlock()

	if !ok {
		if err := k.loadDEK(keyID); err != nil {
			return nil, fmt.Errorf("DEK not found: %s", keyID)
		}
		k.mu.RLock()
		secureWrapped, ok = k.deks[keyID]
		k.mu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("DEK not found: %s", keyID)
		}
	}

	var blobInfo wrapping.BlobInfo
	wrappedData := secureWrapped.Data()
	if err := json.Unmarshal(wrappedData, &blobInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wrapped DEK: %w", err)
	}

	return k.wrapper.Unwrap(k.ctx, &blobInfo)
}

func (k *KMS) loadDEKs() error {
	if k.store == nil {
		return nil
	}
	return k.store.IterateMeta(func(keyID string, wrappedData []byte) error {
		k.mu.Lock()
		k.deks[keyID] = newSecureBytes(wrappedData)
		k.mu.Unlock()
		return nil
	})
}

func (k *KMS) loadDEK(keyID string) error {
	if k.store == nil {
		return fmt.Errorf("no store available")
	}

	wrappedData, err := k.store.GetKeyMeta(keyID)
	if err != nil {
		return fmt.Errorf("failed to get DEK: %w", err)
	}

	k.mu.Lock()
	k.deks[keyID] = newSecureBytes(wrappedData)
	k.mu.Unlock()

	return nil
}
