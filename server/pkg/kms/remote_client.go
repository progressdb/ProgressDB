package kms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// RemoteClient implements KMSProvider over an HTTP-over-unix transport.
type RemoteClient struct {
	addr  string
	httpc *http.Client
}

// NewRemoteClient returns a client bound to addr (unix socket path).
func NewRemoteClient(addr string) *RemoteClient {
	tr := &http.Transport{}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		// Use a Dialer that respects the provided context when dialing the
		// unix domain socket address.
		var d net.Dialer
		return d.DialContext(ctx, "unix", addr)
	}
	client := &http.Client{Transport: tr}
	// Default request timeout to avoid hanging requests
	client.Timeout = 10 * time.Second
	return &RemoteClient{addr: addr, httpc: client}
}

func (r *RemoteClient) Enabled() bool { return true }

func (r *RemoteClient) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	return nil, nil, "", fmt.Errorf("remote client Encrypt: %w", ErrNotImplemented)
}

func (r *RemoteClient) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	return nil, fmt.Errorf("remote client Decrypt: %w", ErrNotImplemented)
}

func (r *RemoteClient) CreateDEK() (string, []byte, string, string, error) {
	return "", nil, "", "", ErrNotImplemented
}
func (r *RemoteClient) WrapDEK(dek []byte) ([]byte, error)       { return nil, ErrNotImplemented }
func (r *RemoteClient) UnwrapDEK(wrapped []byte) ([]byte, error) { return nil, ErrNotImplemented }
func (r *RemoteClient) Health() error                            { return ErrNotImplemented }
func (r *RemoteClient) Close() error                             { return nil }

// Health probes the remote miniKMS service. It expects a 200 response from
// `/healthz` (or `/health`) and returns an error with body text when the
// response status is not OK.
func (r *RemoteClient) HealthCheck() error {
	// try /healthz then /health
	paths := []string{"/healthz", "/health"}
	var lastErr error
	for _, p := range paths {
		url := "http://unix" + p
		req, _ := http.NewRequest("GET", url, nil)
		resp, err := r.httpc.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			return nil
		}
		// include body in error for diagnostics
		return fmt.Errorf("health %s: status %d: %s", p, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("health probe failed: %v", lastErr)
}

// CreateDEKForThread requests the miniKMS to create a DEK for a thread.
func (r *RemoteClient) CreateDEKForThread(threadID string) (string, []byte, string, string, error) {
	req := map[string]string{"thread_id": threadID}
	b, _ := json.Marshal(req)
	url := "http://unix/create_dek_for_thread"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return "", nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, "", "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		KeyID      string `json:"key_id"`
		Wrapped    string `json:"wrapped"`
		KekID      string `json:"kek_id"`
		KekVersion string `json:"kek_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, "", "", err
	}
	wb, err := base64.StdEncoding.DecodeString(out.Wrapped)
	if err != nil {
		return "", nil, "", "", err
	}
	return out.KeyID, wb, out.KekID, out.KekVersion, nil
}

// GetWrapped returns wrapped DEK from remote miniKMS
func (r *RemoteClient) GetWrapped(keyID string) ([]byte, error) {
	url := fmt.Sprintf("http://unix/get_wrapped?key_id=%s", keyID)
	reqq, _ := http.NewRequest("GET", url, nil)
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Wrapped string `json:"wrapped"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Wrapped)
}

// EncryptWithKey delegates encryption to remote miniKMS
func (r *RemoteClient) EncryptWithKey(keyID string, plaintext, aad []byte) ([]byte, []byte, string, error) {
	req := map[string]string{"key_id": keyID, "plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	b, _ := json.Marshal(req)
	url := "http://unix/encrypt"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil, "", err
	}
	ct, err := base64.StdEncoding.DecodeString(out.Ciphertext)
	if err != nil {
		return nil, nil, "", err
	}
	return ct, nil, "v1", nil
}

// DecryptWithKey delegates decryption to remote miniKMS
func (r *RemoteClient) DecryptWithKey(keyID string, ciphertext, iv, aad []byte) ([]byte, error) {
	req := map[string]string{"key_id": keyID, "ciphertext": base64.StdEncoding.EncodeToString(ciphertext)}
	b, _ := json.Marshal(req)
	url := "http://unix/decrypt"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Plaintext string `json:"plaintext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Plaintext)
}

// RewrapKey requests the miniKMS to rewrap the stored DEK identified by keyID
// using the provided new KEK (hex). It returns the new kek_id from the service
// when successful.
func (r *RemoteClient) RewrapKey(keyID, newKEKHex string) (newKekID string, err error) {
    req := map[string]string{"key_id": keyID, "new_kek_hex": newKEKHex}
    b, _ := json.Marshal(req)
    url := "http://unix/rewrap"
    reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
    reqq.Header.Set("Content-Type", "application/json")
    resp, err := r.httpc.Do(reqq)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
    }
    var out struct{
        Status string `json:"status"`
        KeyID string `json:"key_id"`
        KekID string `json:"kek_id"`
    }
    if err := json.Unmarshal(body, &out); err != nil {
        return "", err
    }
    return out.KekID, nil
}
