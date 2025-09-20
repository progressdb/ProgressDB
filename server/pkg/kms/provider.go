package kms

import (
	"errors"
	"progressdb/pkg/logger"
	"sync"
)

var (
	providerMu sync.RWMutex
	provider   KMSProvider
)

// RegisterKMSProvider registers the active provider for the server.
func RegisterKMSProvider(p KMSProvider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = p
}

// UnregisterKMSProvider removes any registered provider.
func UnregisterKMSProvider() {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = nil
}

// IsProviderEnabled reports whether a provider is registered and enabled.
func IsProviderEnabled() bool {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return false
	}
	return p.Enabled()
}

// CreateDEKForThread requests a DEK scoped to the provided threadID via the provider.
func CreateDEKForThread(threadID string) (string, []byte, string, string, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return "", nil, "", "", errors.New("no kms provider registered")
	}
	type threadCreator interface {
		CreateDEKForThread(string) (string, []byte, string, string, error)
	}
	if tc, ok := p.(threadCreator); ok {
		return tc.CreateDEKForThread(threadID)
	}
	return "", nil, "", "", errors.New("provider does not support CreateDEKForThread")
}

// EncryptWithDEK encrypts using a DEK referenced by dekID via the provider.
func EncryptWithDEK(dekID string, plaintext, aad []byte) ([]byte, string, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, "", errors.New("no kms provider registered")
	}
	type encNewIf interface {
		EncryptWithDEK(string, []byte, []byte) ([]byte, string, error)
	}
	if e, ok := p.(encNewIf); ok {
		return e.EncryptWithDEK(dekID, plaintext, aad)
	}
	return nil, "", errors.New("provider does not support EncryptWithDEK")
}

// DecryptWithDEK decrypts a ciphertext blob using the provider.
func DecryptWithDEK(dekID string, ciphertext, aad []byte) ([]byte, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		logger.ErrorKV("DecryptWithDEK failed: no kms provider registered", "dekID", dekID)
		return nil, errors.New("no kms provider registered")
	}
	type decNewIf interface {
		DecryptWithDEK(string, []byte, []byte) ([]byte, error)
	}
	if d, ok := p.(decNewIf); ok {
		plaintext, err := d.DecryptWithDEK(dekID, ciphertext, aad)
		if err != nil {
			logger.ErrorKV("DecryptWithDEK error", "dekID", dekID, "err", err)
		} else {
			logger.InfoKV("DecryptWithDEK success", "dekID", dekID, "plaintext_len", len(plaintext))
		}
		return plaintext, err
	}
	logger.ErrorKV("DecryptWithDEK failed: provider does not support DecryptWithDEK", "dekID", dekID)
	return nil, errors.New("provider does not support DecryptWithDEK")
}

// GetWrappedDEK requests wrapped blob for key id from provider.
func GetWrappedDEK(keyID string) ([]byte, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, errors.New("no kms provider registered")
	}
	type gwIf interface {
		GetWrapped(string) ([]byte, error)
	}
	if g, ok := p.(gwIf); ok {
		return g.GetWrapped(keyID)
	}
	return nil, errors.New("provider does not support GetWrapped")
}

// UnwrapDEK delegates to provider to unwrap a wrapped DEK.
func UnwrapDEK(wrapped []byte) ([]byte, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, errors.New("no kms provider registered")
	}
	type uwIf interface {
		UnwrapDEK([]byte) ([]byte, error)
	}
	if u, ok := p.(uwIf); ok {
		return u.UnwrapDEK(wrapped)
	}
	return nil, errors.New("provider does not support UnwrapDEK")
}

// RewrapDEKForThread asks provider to rewrap an existing DEK under new KEK hex.
func RewrapDEKForThread(dekID string, newKEKHex string) ([]byte, string, string, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, "", "", errors.New("no kms provider registered")
	}
	type rwIf interface {
		RewrapDEKForThread(string, string) ([]byte, string, string, error)
	}
	if rw, ok := p.(rwIf); ok {
		return rw.RewrapDEKForThread(dekID, newKEKHex)
	}
	return nil, "", "", errors.New("provider does not support RewrapDEKForThread")
}
