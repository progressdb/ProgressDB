package embedded

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/progressdb/kms/pkg/config"
	security "github.com/progressdb/kms/pkg/core"
	"github.com/progressdb/kms/pkg/store"
)

// EmbeddedKMS provides in-process KMS functionality without HTTP layer
type EmbeddedKMS struct {
	provider security.KMSProvider
	store    *store.Store
}

// DEK represents a Data Encryption Key with its metadata
type DEK struct {
	ID         string `json:"id"`
	Data       []byte `json:"data,omitempty"`
	Wrapped    []byte `json:"wrapped"`
	KekID      string `json:"kek_id"`
	KekVersion string `json:"kek_version"`
	ThreadKey  string `json:"thread_key,omitempty"`
}

// New creates a new embedded KMS instance using the service-compatible config
func New(ctx context.Context, cfg *config.Config) (*EmbeddedKMS, error) {
	// Get master key using same logic as main config
	masterKey, err := getMasterKeyFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get master key: %w", err)
	}

	// Initialize provider
	provider, err := security.NewHashicorpProviderFromHex(ctx, masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	// Initialize store
	dataPath := cfg.Encryption.KMS.DataDir
	if dataPath == "" {
		dataPath = "./kms/kms.db"
	}

	st, err := store.New(dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	return &EmbeddedKMS{
		provider: provider,
		store:    st,
	}, nil
}

// getMasterKeyFromConfig extracts master key from service-compatible config
func getMasterKeyFromConfig(cfg *config.Config) (string, error) {
	kmsConfig := &cfg.Encryption.KMS

	// Check master key file first
	if kmsConfig.MasterKeyFile != "" {
		keyBytes, err := os.ReadFile(kmsConfig.MasterKeyFile)
		if err != nil {
			return "", fmt.Errorf("failed to read master key file %s: %w", kmsConfig.MasterKeyFile, err)
		}
		keyHex := strings.TrimSpace(string(keyBytes))
		if err := config.ValidateMasterKey(keyHex); err != nil {
			return "", fmt.Errorf("invalid master key in file %s: %w", kmsConfig.MasterKeyFile, err)
		}
		return keyHex, nil
	}

	// Check master key hex
	if kmsConfig.MasterKeyHex != "" {
		if err := config.ValidateMasterKey(kmsConfig.MasterKeyHex); err != nil {
			return "", fmt.Errorf("invalid master_key_hex: %w", err)
		}
		return kmsConfig.MasterKeyHex, nil
	}

	return "", fmt.Errorf("no master key configured: set either master_key_file or master_key_hex")
}

// CreateDEK creates a new Data Encryption Key for the specified thread
func (e *EmbeddedKMS) CreateDEK(threadKey string) (*DEK, error) {
	if !e.provider.Enabled() {
		return nil, fmt.Errorf("KMS provider not enabled")
	}

	dekID, wrapped, kekID, kekVersion, err := e.provider.CreateDEKForThread(threadKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create DEK: %w", err)
	}

	// Save metadata to store
	meta := map[string]string{
		"wrapped":    string(wrapped),
		"thread_key": threadKey,
	}
	metaBytes, _ := json.Marshal(meta)
	_ = e.store.SaveKeyMeta(dekID, metaBytes)

	return &DEK{
		ID:         dekID,
		Wrapped:    wrapped,
		KekID:      kekID,
		KekVersion: kekVersion,
		ThreadKey:  threadKey,
	}, nil
}

// Encrypt encrypts data using the specified DEK
func (e *EmbeddedKMS) Encrypt(dekID string, plaintext []byte) ([]byte, error) {
	if !e.provider.Enabled() {
		return nil, fmt.Errorf("KMS provider not enabled")
	}

	ciphertext, _, err := e.provider.EncryptWithDEK(dekID, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	return ciphertext, nil
}

// Decrypt decrypts data using the specified DEK
func (e *EmbeddedKMS) Decrypt(dekID string, ciphertext []byte) ([]byte, error) {
	if !e.provider.Enabled() {
		return nil, fmt.Errorf("KMS provider not enabled")
	}

	plaintext, err := e.provider.DecryptWithDEK(dekID, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// Rewrap rewraps a DEK with a new key encryption key
func (e *EmbeddedKMS) Rewrap(dekID string, newKEK string) (*DEK, error) {
	if !e.provider.Enabled() {
		return nil, fmt.Errorf("KMS provider not enabled")
	}

	newWrapped, newKekID, newKekVersion, err := e.provider.RewrapDEKForThread(dekID, newKEK)
	if err != nil {
		return nil, fmt.Errorf("rewrap failed: %w", err)
	}

	// Update metadata
	mb, err := e.store.GetKeyMeta(dekID)
	if err != nil {
		return nil, fmt.Errorf("failed to get key metadata: %w", err)
	}

	var m map[string]string
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, fmt.Errorf("invalid key metadata: %w", err)
	}

	meta := map[string]string{
		"wrapped":    string(newWrapped),
		"thread_key": m["thread_key"],
	}
	metaBytes, _ := json.Marshal(meta)
	_ = e.store.SaveKeyMeta(dekID, metaBytes)

	return &DEK{
		ID:         dekID,
		Wrapped:    newWrapped,
		KekID:      newKekID,
		KekVersion: newKekVersion,
		ThreadKey:  m["thread_key"],
	}, nil
}

// GetWrapped returns the wrapped DEK for the specified key ID
func (e *EmbeddedKMS) GetWrapped(dekID string) ([]byte, error) {
	mb, err := e.store.GetKeyMeta(dekID)
	if err != nil {
		return nil, fmt.Errorf("failed to get key metadata: %w", err)
	}

	var m map[string]string
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, fmt.Errorf("invalid key metadata: %w", err)
	}

	return []byte(m["wrapped"]), nil
}

// Enabled returns true if the KMS provider is enabled
func (e *EmbeddedKMS) Enabled() bool {
	return e.provider != nil && e.provider.Enabled()
}

// Health checks the health of the KMS
func (e *EmbeddedKMS) Health() error {
	if e.provider == nil {
		return fmt.Errorf("provider not initialized")
	}
	return e.provider.Health()
}

// Close closes the embedded KMS and cleans up resources
func (e *EmbeddedKMS) Close() error {
	var errs []error

	if e.provider != nil {
		if err := e.provider.Close(); err != nil {
			errs = append(errs, fmt.Errorf("provider close error: %w", err))
		}
	}

	if e.store != nil {
		if err := e.store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("store close error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("multiple errors during close: %v", errs)
	}

	return nil
}
