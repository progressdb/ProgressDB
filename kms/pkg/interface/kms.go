package kmsinterface

// DEK represents a Data Encryption Key with its metadata
type DEK struct {
	ID         string `json:"id"`
	Data       []byte `json:"data,omitempty"`
	Wrapped    []byte `json:"wrapped"`
	KekID      string `json:"kek_id"`
	KekVersion string `json:"kek_version"`
	ThreadID   string `json:"thread_id,omitempty"`
}

// KMS defines the unified interface for both embedded and external KMS operations
type KMS interface {
	// Core encryption operations
	CreateDEK(threadID string) (*DEK, error)
	Encrypt(dekID string, plaintext []byte) ([]byte, error)
	Decrypt(dekID string, ciphertext []byte) ([]byte, error)

	// Key management
	Rewrap(dekID string, newKEK string) (*DEK, error)
	GetWrapped(dekID string) ([]byte, error)

	// Service management
	Enabled() bool
	Health() error
	Close() error
}

// Provider extends KMS with additional provider-specific methods
type Provider interface {
	KMS

	// Provider-specific operations
	CreateDEKForThread(threadID string) (string, []byte, string, string, error)
	EncryptWithDEK(dekID string, plaintext, aad []byte) ([]byte, string, error)
	DecryptWithDEK(dekID string, ciphertext, aad []byte) ([]byte, error)
	RewrapDEKForThread(dekID string, newKEKHex string) ([]byte, string, string, error)

	// Additional methods for external compatibility
	WrapDEK(dek []byte) ([]byte, error)
	UnwrapDEK(wrapped []byte) ([]byte, error)
}

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}
