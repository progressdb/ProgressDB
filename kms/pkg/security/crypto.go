package security

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "strings"
    "sync"
)

// securityRandReadImpl reads cryptographically secure random bytes.
func securityRandReadImpl(b []byte) (int, error) { return rand.Read(b) }

var key []byte
var (
    providerMu sync.RWMutex
    provider   KMSProvider
    keyLocked  bool
)

// KMSProvider mirrors the minimal interface expected by the security layer.
type KMSProvider interface {
    Enabled() bool
    Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error)
    Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error)
    CreateDEK() (keyID string, wrapped []byte, err error)
    CreateDEKForThread(threadID string) (keyID string, wrapped []byte, err error)
    EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error)
    DecryptWithKey(keyID string, ciphertext, iv, aad []byte) (plaintext []byte, err error)
    WrapDEK(dek []byte) ([]byte, error)
    UnwrapDEK(wrapped []byte) ([]byte, error)
    GetWrapped(keyID string) ([]byte, error)
    Health() error
    Close() error
}

func RegisterKMSProvider(p KMSProvider) {
    providerMu.Lock()
    defer providerMu.Unlock()
    provider = p
}

func UnregisterKMSProvider() {
    providerMu.Lock()
    defer providerMu.Unlock()
    provider = nil
}

type EncField struct {
    Path      string
    Algorithm string
}

type fieldRule struct {
    segs      []string
    algorithm string
}

var fieldRules []fieldRule

func SetFieldPolicy(fields []EncField) error {
    fieldRules = fieldRules[:0]
    for _, f := range fields {
        alg := strings.ToLower(strings.TrimSpace(f.Algorithm))
        if alg == "" {
            alg = "aes-gcm"
        }
        if alg != "aes-gcm" {
            return fmt.Errorf("unsupported algorithm: %s", f.Algorithm)
        }
        p := strings.TrimSpace(f.Path)
        if p == "" {
            continue
        }
        segs := strings.Split(p, ".")
        fieldRules = append(fieldRules, fieldRule{segs: segs, algorithm: alg})
    }
    return nil
}

func HasFieldPolicy() bool { return len(fieldRules) > 0 }

func SetKeyHex(hexKey string) error {
    if hexKey == "" {
        if key != nil && keyLocked {
            _ = UnlockMemory(key)
            keyLocked = false
        }
        key = nil
        return nil
    }
    b, err := hex.DecodeString(hexKey)
    if err != nil {
        return err
    }
    if l := len(b); l != 32 {
        return errors.New("encryption key must be 32 bytes (AES-256)")
    }
    if key != nil && keyLocked {
        _ = UnlockMemory(key)
        keyLocked = false
    }
    key = b
    if err := LockMemory(key); err == nil {
        keyLocked = true
    }
    return nil
}

func Enabled() bool {
    providerMu.RLock()
    p := provider
    providerMu.RUnlock()
    if p != nil && p.Enabled() {
        return true
    }
    return len(key) == 32
}

func Encrypt(plaintext []byte) ([]byte, error) {
    providerMu.RLock()
    p := provider
    providerMu.RUnlock()
    if p != nil && p.Enabled() {
        ct, iv, _, err := p.Encrypt(plaintext, nil)
        if err != nil {
            return nil, err
        }
        if iv != nil && len(iv) > 0 {
            return append(iv, ct...), nil
        }
        return ct, nil
    }
    if !Enabled() {
        out := append([]byte(nil), plaintext...)
        return out, nil
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }
    out := gcm.Seal(nil, nonce, plaintext, nil)
    return append(nonce, out...), nil
}

func Decrypt(data []byte) ([]byte, error) {
    providerMu.RLock()
    p := provider
    providerMu.RUnlock()
    if p != nil && p.Enabled() {
        return p.Decrypt(data, nil, nil)
    }
    if !Enabled() {
        return append([]byte(nil), data...), nil
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    ns := gcm.NonceSize()
    if len(data) < ns {
        return nil, errors.New("ciphertext too short")
    }
    nonce := data[:ns]
    ct := data[ns:]
    return gcm.Open(nil, nonce, ct, nil)
}

func CreateDEK() (string, []byte, error) {
    providerMu.RLock()
    p := provider
    providerMu.RUnlock()
    if p == nil {
        return "", nil, errors.New("no kms provider registered")
    }
    return p.CreateDEK()
}

func CreateDEKForThread(threadID string) (string, []byte, error) {
    providerMu.RLock()
    p := provider
    providerMu.RUnlock()
    if p == nil {
        return "", nil, errors.New("no kms provider registered")
    }
    type threadCreator interface {
        CreateDEKForThread(string) (string, []byte, error)
    }
    if tc, ok := p.(threadCreator); ok {
        return tc.CreateDEKForThread(threadID)
    }
    return p.CreateDEK()
}

