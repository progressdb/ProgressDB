package encryption

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"progressdb/pkg/state/telemetry"
)

type RemoteClient struct {
	addr    string
	httpc   *http.Client
	baseURL string
}

func NewRemoteClient(addr string) *RemoteClient {
	var client *http.Client
	base := ""
	client = &http.Client{}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		base = strings.TrimRight(addr, "/")
	} else {
		base = "http://" + strings.TrimRight(addr, "/")
	}
	client.Timeout = 10 * time.Second
	return &RemoteClient{addr: addr, httpc: client, baseURL: base}
}

func (r *RemoteClient) Health() error { return r.HealthCheck() }

func (r *RemoteClient) Close() error  { return nil }
func (r *RemoteClient) Enabled() bool { return true }

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

func (r *RemoteClient) CreateDEK(keyID ...string) (string, []byte, string, string, error) {
	tr := telemetry.Track("kms.remote.create_dek")
	defer tr.Finish()

	var req map[string]interface{}
	if len(keyID) > 0 && keyID[0] != "" {
		req = map[string]interface{}{"key_id": keyID[0]}
	} else {
		req = map[string]interface{}{}
	}

	b, _ := json.Marshal(req)
	url := r.baseURL + "/deks"
	reqq, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	reqq.Header.Set("Content-Type", "application/json")
	resp, err := r.httpc.Do(reqq)
	if err != nil {
		return "", nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, "", "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		KeyID      string          `json:"key_id"`
		WrappedDEK json.RawMessage `json:"wrapped_dek"`
		KekID      string          `json:"kek_id"`
		KekVersion string          `json:"kek_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, "", "", err
	}
	return out.KeyID, out.WrappedDEK, out.KekID, out.KekVersion, nil
}

func (r *RemoteClient) EncryptWithDEK(keyID string, plaintext, aad []byte) ([]byte, string, error) {
	tr := telemetry.Track("kms.remote.encrypt_with_dek")
	defer tr.Finish()

	req := map[string]interface{}{"key_id": keyID, "plaintext": plaintext}
	if aad != nil {
		req["aad"] = aad
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
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Ciphertext json.RawMessage `json:"ciphertext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	return out.Ciphertext, "", nil
}

func (r *RemoteClient) DecryptWithDEK(keyID string, ciphertext, aad []byte) ([]byte, error) {
	tr := telemetry.Track("kms.remote.decrypt_with_dek")
	defer tr.Finish()

	req := map[string]interface{}{"key_id": keyID, "ciphertext": ciphertext}
	if aad != nil {
		req["aad"] = aad
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Plaintext json.RawMessage `json:"plaintext"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Plaintext, nil
}
