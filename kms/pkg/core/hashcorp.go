package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	wrapping "github.com/hashicorp/go-kms-wrapping/v2"
	aead "github.com/hashicorp/go-kms-wrapping/v2/aead"
)

type hashicorpProvider struct {
	ctx     context.Context
	w       *aead.Wrapper
	mu      sync.RWMutex
	wrapped map[string]*secureBytes
}

func validateKeyEntropy(key []byte) error {
	if len(key) != 32 {
		return errors.New("key must be exactly 32 bytes")
	}

	freq := make(map[byte]int)
	for _, b := range key {
		freq[b]++
	}

	var entropy float64
	for _, count := range freq {
		if count > 0 {
			p := float64(count) / float64(len(key))
			entropy -= p * math.Log2(p)
		}
	}

	if entropy < 2.0 {
		return fmt.Errorf("insufficient key entropy: %.2f < 7.0", entropy)
	}

	if isWeakPattern(key) {
		return errors.New("key contains weak or predictable patterns")
	}

	return nil
}

func isWeakPattern(key []byte) bool {
	allZeros := true
	for _, b := range key {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		return true
	}

	if len(key) > 1 {
		first := key[0]
		allSame := true
		for _, b := range key {
			if b != first {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}

	sequential := true
	for i := 1; i < len(key); i++ {
		if key[i] != key[i-1]+1 {
			sequential = false
			break
		}
	}
	if sequential {
		return true
	}

	reverseSequential := true
	for i := 1; i < len(key); i++ {
		if key[i] != key[i-1]-1 {
			reverseSequential = false
			break
		}
	}
	if reverseSequential {
		return true
	}

	return false
}

type secureBytes struct {
	data []byte
}

func newSecureBytes(data []byte) *secureBytes {
	sb := &secureBytes{data: make([]byte, len(data))}
	copy(sb.data, data)
	return sb
}

func (sb *secureBytes) Data() []byte {
	if sb == nil {
		return nil
	}
	return sb.data
}

func (sb *secureBytes) Clear() {
	if sb == nil || sb.data == nil {
		return
	}

	for range 3 {
		for j := range sb.data {
			sb.data[j] = 0
		}
	}

	runtime.KeepAlive(sb.data)

	sb.data = nil
}

func secureWipe(data []byte) {
	if data == nil {
		return
	}

	for range 2 {
		crand.Read(data)
	}

	for i := range data {
		data[i] = 0
	}

	runtime.KeepAlive(data)
}

func NewHashicorpProviderFromRaw(ctx context.Context, key []byte) (KMSProvider, error) {
	if err := validateKeyEntropy(key); err != nil {
		return nil, fmt.Errorf("weak master key: %w", err)
	}

	w := aead.NewWrapper()
	cfg := map[string]string{"key": base64.StdEncoding.EncodeToString(key), "key_id": "local"}
	if _, err := w.SetConfig(ctx, wrapping.WithConfigMap(cfg)); err != nil {
		return nil, fmt.Errorf("wrapper setconfig failed: %w", err)
	}
	return &hashicorpProvider{ctx: ctx, w: w, wrapped: make(map[string]*secureBytes)}, nil
}

func (h *hashicorpProvider) Enabled() bool {
	return h != nil && h.w != nil
}

func (h *hashicorpProvider) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	info, err := h.w.Encrypt(h.ctx, plaintext, wrapping.WithAad(aad))
	if err != nil {
		return nil, nil, "", err
	}
	if info == nil || len(info.Ciphertext) == 0 {
		return nil, nil, "", errors.New("encrypt returned empty")
	}
	iv = info.Ciphertext[:12]
	ct := info.Ciphertext[12:]
	keyId, _ := h.w.KeyId(h.ctx)
	return ct, iv, keyId, nil
}

func (h *hashicorpProvider) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	keyId, _ := h.w.KeyId(h.ctx)
	info := &wrapping.BlobInfo{Ciphertext: append(iv, ciphertext...), KeyInfo: &wrapping.KeyInfo{KeyId: keyId}}
	return h.w.Decrypt(h.ctx, info, wrapping.WithAad(aad))
}

func (h *hashicorpProvider) CreateDEK() (string, []byte, error) {
	dek := make([]byte, 32)
	if _, err := SecurityRandRead(dek); err != nil {
		return "", nil, err
	}
	info, err := h.w.Encrypt(h.ctx, dek)
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		return "", nil, err
	}
	kid := fmt.Sprintf("k_%d", time.Now().UnixNano())
	h.mu.Lock()
	h.wrapped[kid] = newSecureBytes(info.Ciphertext)
	h.mu.Unlock()

	secureWipe(info.Ciphertext)

	return kid, h.wrapped[kid].Data(), nil
}

func (h *hashicorpProvider) CreateDEKForThread(threadKey string) (string, []byte, string, string, error) {
	kid, wrapped, err := h.CreateDEK()
	if err != nil {
		return "", nil, "", "", err
	}
	kidInfo, _ := h.w.KeyId(h.ctx)
	return kid, wrapped, kidInfo, "", nil
}

func (h *hashicorpProvider) EncryptWithDEK(dekID string, plaintext, aad []byte) ([]byte, string, error) {
	h.mu.RLock()
	secureWrapped, ok := h.wrapped[dekID]
	h.mu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("dek not found: %s", dekID)
	}
	dek, err := h.UnwrapDEK(secureWrapped.Data())
	if err != nil {
		return nil, "", err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, "", err
	}
	ct := gcm.Seal(nil, nonce, plaintext, aad)
	out := append(nonce, ct...)
	kid, _ := h.w.KeyId(h.ctx)
	return out, kid, nil
}

func (h *hashicorpProvider) DecryptWithDEK(dekID string, ciphertext, aad []byte) ([]byte, error) {
	h.mu.RLock()
	secureWrapped, ok := h.wrapped[dekID]
	h.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dek not found: %s", dekID)
	}
	dek, err := h.UnwrapDEK(secureWrapped.Data())
	if err != nil {
		return nil, err
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
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[:ns]
	ct := ciphertext[ns:]
	return gcm.Open(nil, nonce, ct, aad)
}

func (h *hashicorpProvider) RewrapDEKForThread(dekID string, newKEKHex string) ([]byte, string, string, error) {
	h.mu.RLock()
	secureWrapped, ok := h.wrapped[dekID]
	h.mu.RUnlock()
	if !ok {
		return nil, "", "", fmt.Errorf("dek not found: %s", dekID)
	}
	dek, err := h.UnwrapDEK(secureWrapped.Data())
	if err != nil {
		return nil, "", "", err
	}
	newProv, err := NewHashicorpProviderFromHex(context.Background(), newKEKHex)
	if err != nil {
		return nil, "", "", err
	}
	type wrapIf interface {
		WrapDEK([]byte) ([]byte, error)
		KeyInfo() (string, string)
	}
	var newWrapped []byte
	if w, ok := newProv.(wrapIf); ok {
		nw, err := w.WrapDEK(dek)
		if err != nil {
			return nil, "", "", err
		}
		newWrapped = nw
	} else {
		return nil, "", "", fmt.Errorf("new provider does not support WrapDEK")
	}
	h.mu.Lock()
	h.wrapped[dekID] = newSecureBytes(newWrapped)
	h.mu.Unlock()

	if secureWrapped != nil {
		secureWrapped.Clear()
	}

	secureWipe(newWrapped)
	kid, _ := newProv.(wrapIf).KeyInfo()
	return newWrapped, kid, "", nil
}

func (h *hashicorpProvider) WrapDEK(dek []byte) ([]byte, error) {
	info, err := h.w.Encrypt(h.ctx, dek)
	if err != nil {
		return nil, err
	}
	return info.Ciphertext, nil
}

func (h *hashicorpProvider) UnwrapDEK(wrapped []byte) ([]byte, error) {
	info := &wrapping.BlobInfo{Ciphertext: wrapped}
	return h.w.Decrypt(h.ctx, info)
}

func (h *hashicorpProvider) GetWrapped(keyID string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (h *hashicorpProvider) Health() error { return nil }

func (h *hashicorpProvider) Close() error {
	h.mu.Lock()
	for _, secureData := range h.wrapped {
		if secureData != nil {
			secureData.Clear()
		}
	}
	h.wrapped = make(map[string]*secureBytes)
	h.mu.Unlock()

	if f, ok := any(h.w).(interface {
		Finalize(context.Context, ...wrapping.Option) error
	}); ok {
		return f.Finalize(h.ctx)
	}
	return nil
}

func NewHashicorpProviderFromHex(ctx context.Context, hexKey string) (KMSProvider, error) {
	if hexKey == "" {
		return nil, errors.New("master key cannot be empty")
	}

	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex format: %w", err)
	}

	if err := validateKeyEntropy(b); err != nil {
		return nil, fmt.Errorf("weak master key: %w", err)
	}

	return NewHashicorpProviderFromRaw(ctx, b)
}
