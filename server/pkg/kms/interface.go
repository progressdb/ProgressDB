package kms

import "errors"

// ErrNotImplemented is returned by stub methods that are not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// KMSProvider defines the operations the server expects from a KMS.
type KMSProvider interface {
	// Enabled reports whether the provider is available and should be used.
	Enabled() bool

	// Encrypt encrypts plaintext with optional associated data. The
	// returned ciphertext format is provider-specific; callers should treat
	// the returned iv as optional (nil when ciphertext already contains it).
	Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error)

	// Decrypt returns the plaintext for the given ciphertext/iv and AAD.
	Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error)

	// CreateDEK generates a new data-encryption-key and returns an opaque
	// key identifier and the wrapped key material.
	CreateDEK() (keyID string, wrapped []byte, err error)
	// CreateDEKForThread creates a DEK scoped to a specific thread id.
	CreateDEKForThread(threadID string) (keyID string, wrapped []byte, err error)
	// EncryptWithKey encrypts plaintext under the DEK identified by keyID.
	EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error)
	// DecryptWithKey decrypts ciphertext under the DEK identified by keyID.
	DecryptWithKey(keyID string, ciphertext, iv, aad []byte) (plaintext []byte, err error)

	// WrapDEK wraps a raw DEK under the provider's master key.
	WrapDEK(dek []byte) ([]byte, error)

	// UnwrapDEK unwraps a wrapped DEK and returns the raw DEK bytes.
	UnwrapDEK(wrapped []byte) ([]byte, error)

	// GetWrapped returns the wrapped DEK blob for a key id managed by the provider.
	GetWrapped(keyID string) ([]byte, error)

	// Health returns nil when the KMS is ready to serve requests.
	Health() error

	// Close releases any resources held by the provider.
	Close() error
}
