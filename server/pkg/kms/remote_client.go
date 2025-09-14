package kms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
		return net.Dial("unix", addr)
	}
	return &RemoteClient{addr: addr, httpc: &http.Client{Transport: tr}}
}

func (r *RemoteClient) Enabled() bool { return true }

func (r *RemoteClient) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	return nil, nil, "", fmt.Errorf("remote client Encrypt: %w", ErrNotImplemented)
}

func (r *RemoteClient) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	return nil, fmt.Errorf("remote client Decrypt: %w", ErrNotImplemented)
}

func (r *RemoteClient) CreateDEK() (string, []byte, error)       { return "", nil, ErrNotImplemented }
func (r *RemoteClient) WrapDEK(dek []byte) ([]byte, error)       { return nil, ErrNotImplemented }
func (r *RemoteClient) UnwrapDEK(wrapped []byte) ([]byte, error) { return nil, ErrNotImplemented }
func (r *RemoteClient) Health() error                            { return ErrNotImplemented }
func (r *RemoteClient) Close() error                             { return nil }

// CreateDEKForThread requests the miniKMS to create a DEK for a thread.
func (r *RemoteClient) CreateDEKForThread(threadID string) (string, []byte, error) {
	req := map[string]string{"thread_id": threadID}
	b, _ := json.Marshal(req)
	url := "http://unix/create_dek_for_thread"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		KeyID, Wrapped string `json:"key_id","wrapped"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, err
	}
	wb, err := base64.StdEncoding.DecodeString(out.Wrapped)
	if err != nil {
		return "", nil, err
	}
	return out.KeyID, wb, nil
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
