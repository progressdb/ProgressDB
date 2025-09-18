package kms

// KMSProvider is the minimal interface used by the server to interact with
// an external KMS provider (KMS or others). Implementations may live in
// separate projects/repositories.
type KMSProvider interface {
	Enabled() bool
	Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error)
	Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error)
	CreateDEK() (keyID string, wrapped []byte, kekID string, kekVersion string, err error)
	CreateDEKForThread(threadID string) (keyID string, wrapped []byte, kekID string, kekVersion string, err error)
	EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error)
	DecryptWithKey(keyID string, ciphertext, iv, aad []byte) (plaintext []byte, err error)
	WrapDEK(dek []byte) ([]byte, error)
	UnwrapDEK(wrapped []byte) ([]byte, error)
	GetWrapped(keyID string) ([]byte, error)
	Health() error
	Close() error
}
