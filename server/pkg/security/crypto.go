package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"encoding/json"
	"progressdb/pkg/kmsapi"
)

// securityRandReadImpl reads cryptographically secure random bytes.
func securityRandReadImpl(b []byte) (int, error) { return rand.Read(b) }

var key []byte
var (
	providerMu sync.RWMutex
	provider   kmsapi.KMSProvider
	keyLocked  bool
)

// KMSProvider mirrors the minimal interface expected by the security layer.
// Implementations may be provided by the kms package and registered at
// runtime via RegisterKMSProvider.

// RegisterKMSProvider registers a provider for use by the security package.
func RegisterKMSProvider(p kmsapi.KMSProvider) {
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

type EncField struct {
	Path      string
	Algorithm string
}

type fieldRule struct {
	segs      []string
	algorithm string
}

var fieldRules []fieldRule

// SetFieldPolicy configures selective field encryption paths.
// Only algorithm "aes-gcm" is supported for now.
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

// HasFieldPolicy returns true if selective field encryption is configured.
func HasFieldPolicy() bool { return len(fieldRules) > 0 }

// SetKeyHex sets the AES-256-GCM key from a hex string.
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
	// If an existing key is present and locked, unlock it first.
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

// Enabled returns true if encryption key is configured.
func Enabled() bool {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	// Require a registered provider for production operation. Relying on an
	// in-process master key is deprecated when deploying with an external
	// KMS: the server should fail-fast if no provider is registered.
	if p != nil && p.Enabled() {
		return true
	}
	// Fallback: allow an in-process master key (AES-256) when configured.
	// This enables an "embedded" KMS mode where the server holds the
	// master key instead of talking to an external daemon.
	if key != nil && len(key) == 32 {
		return true
	}
	return false
}

// Encrypt returns nonce|ciphertext using AES-256-GCM.
func Encrypt(plaintext []byte) ([]byte, error) {
	// Delegate to registered KMS provider when present.
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p != nil && p.Enabled() {
		ct, iv, _, err := p.Encrypt(plaintext, nil)
		if err != nil {
			return nil, err
		}
		// If provider returned a separate iv, append or combine as needed.
		// Providers typically return a nonce|ciphertext blob and iv==nil.
		if iv != nil && len(iv) > 0 {
			return append(iv, ct...), nil
		}
		return ct, nil
	}
	if !Enabled() {
		// No-op: return copy of plaintext
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
	// Prepend nonce for storage
	return append(nonce, out...), nil
}

// Decrypt expects nonce|ciphertext.
func Decrypt(data []byte) ([]byte, error) {
	// Delegate to registered provider when present.
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p != nil && p.Enabled() {
		// provider.Decrypt accepts ciphertext, iv, aad. Our adapter assumes
		// ciphertext may already include nonce prefix; pass iv=nil.
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

// CreateDEK delegates to the registered provider to create a new DEK.
func CreateDEK() (string, []byte, string, string, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return "", nil, "", "", errors.New("no kms provider registered")
	}
	// provider.CreateDEK now returns kek metadata
	type createIf interface {
		CreateDEK() (string, []byte, string, string, error)
	}
	if c, ok := p.(createIf); ok {
		return c.CreateDEK()
	}
	return "", nil, "", "", errors.New("provider does not support CreateDEK with kek metadata")
}

// CreateDEKForThread requests a DEK scoped to the provided threadID.
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
	// fallback to generic CreateDEK
	return CreateDEK()
}

// EncryptWithKey delegates an encryption request to the registered KMS
// provider using a DEK referenced by keyID.
func EncryptWithKey(keyID string, plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, nil, "", errors.New("no kms provider registered")
	}
	type encIf interface {
		EncryptWithKey(string, []byte, []byte) ([]byte, []byte, string, error)
	}
	if e, ok := p.(encIf); ok {
		return e.EncryptWithKey(keyID, plaintext, aad)
	}
	return nil, nil, "", errors.New("provider does not support EncryptWithKey")
}

// DecryptWithKey delegates decryption to the registered KMS provider.
func DecryptWithKey(keyID string, ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, errors.New("no kms provider registered")
	}
	type decIf interface {
		DecryptWithKey(string, []byte, []byte, []byte) ([]byte, error)
	}
	if d, ok := p.(decIf); ok {
		return d.DecryptWithKey(keyID, ciphertext, iv, aad)
	}
	return nil, errors.New("provider does not support DecryptWithKey")
}

// GetWrappedDEK returns the wrapped DEK blob for a given key id.
func GetWrappedDEK(keyID string) ([]byte, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, errors.New("no kms provider registered")
	}
	return p.GetWrapped(keyID)
}

// UnwrapDEK delegates to provider to unwrap a wrapped DEK.
func UnwrapDEK(wrapped []byte) ([]byte, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		return nil, errors.New("no kms provider registered")
	}
	return p.UnwrapDEK(wrapped)
}

// EncryptWithRawKey performs AES-256-GCM encryption using the provided raw key (DEK).
// It returns nonce|ciphertext (nonce prepended) similar to Encrypt.
func EncryptWithRawKey(dek, plaintext []byte) ([]byte, error) {
	if len(dek) != 32 {
		return nil, errors.New("invalid DEK length")
	}
	block, err := aes.NewCipher(dek)
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

// DecryptWithRawKey decrypts a nonce|ciphertext blob using the provided raw DEK.
func DecryptWithRawKey(dek, data []byte) ([]byte, error) {
	if len(dek) != 32 {
		return nil, errors.New("invalid DEK length")
	}
	block, err := aes.NewCipher(dek)
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

// EncryptWithKeyBytes wraps provided plaintext bytes using the supplied KEK
// bytes using AES-GCM. Returns nonce|ciphertext blob.
func EncryptWithKeyBytes(kek, plaintext []byte) ([]byte, error) {
	if len(kek) != 32 {
		return nil, errors.New("kek must be 32 bytes")
	}
	block, err := aes.NewCipher(kek)
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

// AuditSign returns a base64 HMAC-SHA256 signature of the provided message
// using the master KEK when available. Returns error if master key not set.
func AuditSign(msg []byte) (string, error) {
	if !Enabled() || key == nil {
		return "", errors.New("master key not configured")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// envelope represents an encrypted JSON field value.
type envelope struct {
	Enc string `json:"_enc"`
	V   string `json:"v"`
}

// EncryptJSONFields encrypts configured JSON paths within the provided JSON bytes.
// Returns the modified JSON if parsing/encryption succeeds.
func EncryptJSONFields(jsonBytes []byte) ([]byte, error) {
	if !Enabled() || !HasFieldPolicy() {
		return append([]byte(nil), jsonBytes...), nil
	}
	// Quick sanity: must look like JSON object or array
	if !looksLikeJSON(jsonBytes) {
		return nil, errors.New("not json")
	}
	var v interface{}
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		return nil, err
	}
	for _, rule := range fieldRules {
		v = encryptAt(v, rule.segs)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DecryptJSONFields decrypts any envelope objects found in JSON.
func DecryptJSONFields(jsonBytes []byte) ([]byte, error) {
	if !Enabled() || !HasFieldPolicy() {
		return append([]byte(nil), jsonBytes...), nil
	}
	if !looksLikeJSON(jsonBytes) {
		return nil, errors.New("not json")
	}
	var v interface{}
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		return nil, err
	}
	v = decryptAll(v)
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func looksLikeJSON(b []byte) bool {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r == '{' || r == '['
}

func encryptAt(node interface{}, segs []string) interface{} {
	if len(segs) == 0 {
		// Encrypt current node value as JSON bytes and wrap in envelope.
		raw, err := json.Marshal(node)
		if err != nil {
			return node
		}
		ct, err := Encrypt(raw)
		if err != nil {
			return node
		}
		return map[string]interface{}{
			"_enc": "gcm",
			"v":    base64.StdEncoding.EncodeToString(ct),
		}
	}
	switch cur := node.(type) {
	case map[string]interface{}:
		seg := segs[0]
		if seg == "*" {
			for k, child := range cur {
				cur[k] = encryptAt(child, segs[1:])
			}
			return cur
		}
		if child, ok := cur[seg]; ok {
			cur[seg] = encryptAt(child, segs[1:])
		}
		return cur
	case []interface{}:
		seg := segs[0]
		if seg == "*" {
			for i, child := range cur {
				cur[i] = encryptAt(child, segs[1:])
			}
			return cur
		}
		if idx, err := strconv.Atoi(seg); err == nil {
			if idx >= 0 && idx < len(cur) {
				cur[idx] = encryptAt(cur[idx], segs[1:])
			}
		}
		return cur
	default:
		return node
	}
}

func decryptAll(node interface{}) interface{} {
	switch cur := node.(type) {
	case map[string]interface{}:
		// Check for envelope directly
		if encType, ok := cur["_enc"].(string); ok {
			if encType == "gcm" {
				if sv, ok := cur["v"].(string); ok {
					if raw, err := base64.StdEncoding.DecodeString(sv); err == nil {
						if pt, err := Decrypt(raw); err == nil {
							// Replace with parsed JSON
							var out interface{}
							if err := json.Unmarshal(pt, &out); err == nil {
								return decryptAll(out)
							}
						}
					}
				}
			}
		}
		// Recurse into map
		for k, v := range cur {
			cur[k] = decryptAll(v)
		}
		return cur
	case []interface{}:
		for i, v := range cur {
			cur[i] = decryptAll(v)
		}
		return cur
	default:
		return node
	}
}
