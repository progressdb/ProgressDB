package kms

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RemoteClient implements KMSProvider over an HTTP-over-unix transport.
type RemoteClient struct {
	addr    string
	httpc   *http.Client
	baseURL string
}

// NewRemoteClient returns a client bound to addr (host:port or full URL).
func NewRemoteClient(addr string) *RemoteClient {
	// Determine transport based on addr format. Support:
	// - plain host:port (e.g. "127.0.0.1:8080")
	// - full URL (http://host:port)
	var client *http.Client
	base := ""
	// Accept either a full URL (`http://host:port`) or a host:port string.
	client = &http.Client{}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		base = strings.TrimRight(addr, "/")
	} else {
		base = "http://" + strings.TrimRight(addr, "/")
	}
	client.Timeout = 10 * time.Second
	return &RemoteClient{addr: addr, httpc: client, baseURL: base}
}

func (r *RemoteClient) Enabled() bool { return true }

func (r *RemoteClient) Encrypt(plaintext, aad []byte) (ciphertext, iv []byte, keyVersion string, err error) {
	return nil, nil, "", fmt.Errorf("remote client Encrypt: not implemented")
}

func (r *RemoteClient) Decrypt(ciphertext, iv, aad []byte) (plaintext []byte, err error) {
	return nil, fmt.Errorf("remote client Decrypt: not implemented")
}

func (r *RemoteClient) CreateDEK() (string, []byte, string, string, error) {
	return "", nil, "", "", fmt.Errorf("not implemented")
}
func (r *RemoteClient) WrapDEK(dek []byte) ([]byte, error) { return nil, fmt.Errorf("not implemented") }
func (r *RemoteClient) UnwrapDEK(wrapped []byte) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *RemoteClient) Health() error { return r.HealthCheck() }
func (r *RemoteClient) Close() error  { return nil }

func (r *RemoteClient) HealthCheck() error {
	paths := []string{"/healthz", "/health"}
	var lastErr error
	for _, p := range paths {
		url := r.baseURL + p
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
		return fmt.Errorf("health %s: status %d: %s", p, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("health probe failed: %v", lastErr)
}

func (r *RemoteClient) CreateDEKForThread(threadID string) (string, []byte, string, string, error) {
	req := map[string]string{"thread_id": threadID}
	b, _ := json.Marshal(req)
	url := r.baseURL + "/create_dek_for_thread"
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

func (r *RemoteClient) GetWrapped(keyID string) ([]byte, error) {
	url := fmt.Sprintf("%s/get_wrapped?key_id=%s", r.baseURL, keyID)
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

// EncryptWithDEK posts plaintext to the remote KMS and returns a single
// ciphertext blob (nonce||ct) decoded from base64, plus an optional
// keyVersion string.
func (r *RemoteClient) EncryptWithDEK(keyID string, plaintext, aad []byte) ([]byte, string, error) {
	req := map[string]string{"key_id": keyID, "plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	if aad != nil {
		req["aad"] = base64.StdEncoding.EncodeToString(aad)
	}
	b, _ := json.Marshal(req)
	url := r.baseURL + "/encrypt"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Ciphertext string `json:"ciphertext"`
		KeyVersion string `json:"key_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	ct, err := base64.StdEncoding.DecodeString(out.Ciphertext)
	if err != nil {
		return nil, "", err
	}
	return ct, out.KeyVersion, nil
}

// DecryptWithDEK requests decryption of a ciphertext blob from the remote
// KMS. The ciphertext is supplied as raw bytes and will be base64-encoded
// at the wire boundary.
func (r *RemoteClient) DecryptWithDEK(keyID string, ciphertext, aad []byte) ([]byte, error) {
	req := map[string]string{"key_id": keyID, "ciphertext": base64.StdEncoding.EncodeToString(ciphertext)}
	if aad != nil {
		req["aad"] = base64.StdEncoding.EncodeToString(aad)
	}
	b, _ := json.Marshal(req)
	url := r.baseURL + "/decrypt"
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

func (r *RemoteClient) RewrapDEKForThread(keyID, newKEKHex string) ([]byte, string, string, error) {
	req := map[string]string{"key_id": keyID, "new_kek_hex": newKEKHex}
	b, _ := json.Marshal(req)
	url := r.baseURL + "/rewrap"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, "", "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		KeyID      string `json:"key_id"`
		Wrapped    string `json:"wrapped"`
		KekID      string `json:"kek_id"`
		KekVersion string `json:"kek_version"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "", "", err
	}
	wb, err := base64.StdEncoding.DecodeString(out.Wrapped)
	if err != nil {
		return nil, "", "", err
	}
	return wb, out.KekID, out.KekVersion, nil
}
