package kms

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"progressdb/pkg/security"
	"progressdb/pkg/store"
)

// KeyMeta holds basic metadata for a DEK managed by the local provider.
type KeyMeta struct {
	ID        string
	CreatedAt time.Time
	Status    string // e.g., "active", "retired"
	Wrapped   []byte // wrapped DEK bytes
	Version   int
	ThreadID  string // associated thread id (optional)
}

// LocalProvider adapts the existing in-process security primitives into a
// KMSProvider implementation suitable for dev/embedded mode. It stores key
// metadata in-memory only.
type LocalProvider struct {
	mu   sync.RWMutex
	keys map[string]*KeyMeta
	// cache maps wrapped-key-hex -> cached raw DEK and expiry
	cacheMu sync.RWMutex
	cache   map[string]struct {
		dek    []byte
		expiry time.Time
	}
	janitorStop chan struct{}
	janitorWg   sync.WaitGroup
}

// NewLocalProvider returns a provider that delegates to pkg/security.
func NewLocalProvider() *LocalProvider {
	p := &LocalProvider{keys: map[string]*KeyMeta{}, cache: map[string]struct {
		dek    []byte
		expiry time.Time
	}{}, janitorStop: make(chan struct{})}
	// Attempt to load persisted keys from store; ignore errors to stay
	// functional when DB is not opened (e.g., in unit tests).
	if ks, err := store.ListKeys("kms:dek:"); err == nil {
		for _, k := range ks {
			if v, err := store.GetKey(k); err == nil {
				var meta KeyMeta
				if json.Unmarshal([]byte(v), &meta) == nil {
					p.keys[meta.ID] = &meta
				}
			}
		}
	}
	return p
}

// StartJanitor begins a background goroutine that evicts expired cache entries.
// This is exported so callers that create a provider can opt-in to background
// cache maintenance.
func (l *LocalProvider) StartJanitor() {
	l.janitorWg.Add(1)
	go func() {
		defer l.janitorWg.Done()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				l.cacheMu.Lock()
				for k, v := range l.cache {
					if now.After(v.expiry) {
						// unlock and zeroize
						_ = security.UnlockMemory(v.dek)
						for i := range v.dek {
							v.dek[i] = 0
						}
						delete(l.cache, k)
					}
				}
				l.cacheMu.Unlock()
			case <-l.janitorStop:
				return
			}
		}
	}()
}

func (l *LocalProvider) Enabled() bool { return security.Enabled() }

// Encrypt delegates to pkg/security.Encrypt. The returned ciphertext is the
// nonce|ct blob produced by that function. iv will be nil in this adapter.
func (l *LocalProvider) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	// Default provider-level encrypt is not used for per-thread DEKs.
	return nil, nil, "", ErrNotImplemented
}

// Decrypt delegates to pkg/security.Decrypt. This adapter ignores iv and uses
// the single-blob format produced by Encrypt.
func (l *LocalProvider) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	return nil, ErrNotImplemented
}

// helper: generate random ID hex
func genID(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateDEK generates a new random 32-byte DEK, wraps it with the master key
// provided via pkg/security, stores metadata in-memory, and returns the key id
// and wrapped bytes.
func (l *LocalProvider) CreateDEK() (string, []byte, error) {
	if !security.Enabled() {
		return "", nil, errors.New("master key not configured")
	}
	// generate raw DEK
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return "", nil, err
	}
	// wrap using security.Encrypt (produces nonce|ct)
	wrapped, err := security.Encrypt(dek)
	if err != nil {
		// zeroize raw dek before returning
		for i := range dek {
			dek[i] = 0
		}
		return "", nil, err
	}
	// cache raw DEK for a short TTL so callers can use it without immediate unwrap
	cached := make([]byte, len(dek))
	copy(cached, dek)
	// zeroize raw dek
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		return "", nil, err
	}
	id, err := genID(16)
	if err != nil {
		return "", nil, err
	}
	meta := &KeyMeta{ID: id, CreatedAt: time.Now().UTC(), Status: "active", Wrapped: append([]byte(nil), wrapped...), Version: 1, ThreadID: ""}
	l.mu.Lock()
	l.keys[id] = meta
	l.mu.Unlock()
	// persist metadata to store if available
	if b, jerr := json.Marshal(meta); jerr == nil {
		_ = store.SaveKey("kms:dek:"+id, b)
	}
	// put cached copy into cache, lock memory when possible
	if len(cached) > 0 {
		_ = security.LockMemory(cached)
		k := hex.EncodeToString(wrapped)
		l.cacheMu.Lock()
		l.cache[k] = struct {
			dek    []byte
			expiry time.Time
		}{dek: cached, expiry: time.Now().Add(5 * time.Minute)}
		l.cacheMu.Unlock()
	}
	return id, append([]byte(nil), wrapped...), nil
}

// GetWrapped returns the wrapped DEK for a given key id if present.
func (l *LocalProvider) GetWrapped(keyID string) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if m, ok := l.keys[keyID]; ok {
		return append([]byte(nil), m.Wrapped...), nil
	}
	return nil, errors.New("key not found")
}

// CreateDEKForThread creates a DEK scoped to the provided threadID and
// persists the mapping so future lookups can find the DEK for the thread.
func (l *LocalProvider) CreateDEKForThread(threadID string) (string, []byte, error) {
	id, wrapped, err := l.CreateDEK()
	if err != nil {
		return "", nil, err
	}
	// update metadata with threadID
	l.mu.Lock()
	if m, ok := l.keys[id]; ok {
		m.ThreadID = threadID
		// persist updated meta
		if b, jerr := json.Marshal(m); jerr == nil {
			_ = store.SaveKey("kms:dek:"+id, b)
		}
	}
	l.mu.Unlock()
	// persist thread->key mapping
	_ = store.SaveThreadKey(threadID, id)
	return id, wrapped, nil
}

// WrapDEK wraps a raw DEK using the master key.
func (l *LocalProvider) WrapDEK(dek []byte) ([]byte, error) {
	if !security.Enabled() {
		return nil, errors.New("master key not configured")
	}
	// copy input to avoid mutating caller slice
	dcopy := append([]byte(nil), dek...)
	wrapped, err := security.Encrypt(dcopy)
	// zeroize copy
	for i := range dcopy {
		dcopy[i] = 0
	}
	if err != nil {
		return nil, err
	}
	// cache the raw dek against the wrapped blob for quick unwraps
	if wrapped != nil {
		k := hex.EncodeToString(wrapped)
		cpy := make([]byte, len(dek))
		copy(cpy, dek)
		_ = security.LockMemory(cpy)
		l.cacheMu.Lock()
		l.cache[k] = struct {
			dek    []byte
			expiry time.Time
		}{dek: cpy, expiry: time.Now().Add(5 * time.Minute)}
		l.cacheMu.Unlock()
	}
	return wrapped, nil
}

// UnwrapDEK unwraps a wrapped DEK and returns the raw DEK bytes. Caller is
// responsible for zeroizing the returned slice when finished using it.
func (l *LocalProvider) UnwrapDEK(wrapped []byte) ([]byte, error) {
	if !security.Enabled() {
		return nil, errors.New("master key not configured")
	}
	if wrapped == nil {
		return nil, errors.New("wrapped key empty")
	}
	k := hex.EncodeToString(wrapped)
	// check cache
	l.cacheMu.RLock()
	if ent, ok := l.cache[k]; ok {
		if time.Now().Before(ent.expiry) {
			// return a copy to the caller
			out := make([]byte, len(ent.dek))
			copy(out, ent.dek)
			l.cacheMu.RUnlock()
			return out, nil
		}
	}
	l.cacheMu.RUnlock()

	// not cached or expired: decrypt with master key
	dek, err := security.Decrypt(wrapped)
	if err != nil {
		return nil, err
	}
	// store in cache for TTL
	cpy := make([]byte, len(dek))
	copy(cpy, dek)
	l.cacheMu.Lock()
	l.cache[k] = struct {
		dek    []byte
		expiry time.Time
	}{dek: cpy, expiry: time.Now().Add(5 * time.Minute)}
	l.cacheMu.Unlock()
	return dek, nil
}

// EncryptWithKey encrypts plaintext under a DEK identified by keyID. It
// unwraps the DEK internally, performs AES-GCM encryption, zeroizes the
// raw DEK, and returns the nonce|ciphertext blob.
func (l *LocalProvider) EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	// find wrapped DEK
	wrapped, err := l.GetWrapped(keyID)
	if err != nil {
		return nil, nil, "", err
	}
	dek, err := l.UnwrapDEK(wrapped)
	if err != nil {
		return nil, nil, "", err
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	ct, err := security.EncryptWithRawKey(dek, plaintext)
	if err != nil {
		return nil, nil, "", err
	}
	return ct, nil, "v1", nil
}

// DecryptWithKey decrypts ciphertext under the DEK identified by keyID.
func (l *LocalProvider) DecryptWithKey(keyID string, ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	wrapped, err := l.GetWrapped(keyID)
	if err != nil {
		return nil, err
	}
	dek, err := l.UnwrapDEK(wrapped)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	return security.DecryptWithRawKey(dek, ciphertext)
}

func (l *LocalProvider) Health() error {
	if !security.Enabled() {
		return errors.New("local kms: master key not configured")
	}
	return nil
}

func (l *LocalProvider) Close() error {
	// zeroize cached DEKs and clear
	// stop janitor
	if l.janitorStop != nil {
		close(l.janitorStop)
		l.janitorWg.Wait()
	}
	l.clearCache()
	// zeroize wrapped blobs in metadata
	l.mu.Lock()
	for _, m := range l.keys {
		for i := range m.Wrapped {
			m.Wrapped[i] = 0
		}
		m.Wrapped = nil
	}
	l.keys = map[string]*KeyMeta{}
	l.mu.Unlock()
	return nil
}

// clearCache zeroizes cached DEKs and clears the cache map.
func (l *LocalProvider) clearCache() {
	l.cacheMu.Lock()
	for k, v := range l.cache {
		_ = security.UnlockMemory(v.dek)
		for i := range v.dek {
			v.dek[i] = 0
		}
		delete(l.cache, k)
	}
	l.cacheMu.Unlock()
}
