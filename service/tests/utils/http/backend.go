package httputil

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"testing"
)

// DoBackendRequest performs an HTTP request using the provided backend API key
// and signing secret. If user is non-empty, it will set X-User-ID and
// X-User-Signature signed with the signingSecret. Returns the response and
// body bytes.
func DoBackendRequest(t *testing.T, baseURL, method, path string, body interface{}, user, backendAPIKey, signingSecret string) (*nethttp.Response, []byte) {
	t.Helper()
	var rb io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
		rb = bytes.NewReader(b)
	}
	req, err := nethttp.NewRequest(method, baseURL+path, rb)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-User-ID", user)
		req.Header.Set("X-User-Signature", SignHMAC(signingSecret, user))
	}
	if backendAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+backendAPIKey)
	}
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	// reset Body so callers can still read if they want
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return resp, bodyBytes
}

// DoBackendJSON performs a backend request and decodes the JSON response into out.
// It returns the HTTP status code.
func DoBackendJSON(t *testing.T, baseURL, method, path string, body interface{}, user, backendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	resp, data := DoBackendRequest(t, baseURL, method, path, body, user, backendAPIKey, signingSecret)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

func BackendGetJSON(t *testing.T, baseURL, path, user, backendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	return DoBackendJSON(t, baseURL, "GET", path, nil, user, backendAPIKey, signingSecret, out)
}

func BackendPostJSON(t *testing.T, baseURL, path string, reqBody interface{}, user, backendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	return DoBackendJSON(t, baseURL, "POST", path, reqBody, user, backendAPIKey, signingSecret, out)
}

func BackendPutJSON(t *testing.T, baseURL, path string, reqBody interface{}, user, backendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	return DoBackendJSON(t, baseURL, "PUT", path, reqBody, user, backendAPIKey, signingSecret, out)
}

func BackendDeleteJSON(t *testing.T, baseURL, path string, user, backendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	return DoBackendJSON(t, baseURL, "DELETE", path, nil, user, backendAPIKey, signingSecret, out)
}

func BackendRequest(t *testing.T, baseURL, method, path string, body interface{}, user, backendAPIKey, signingSecret string) (int, []byte) {
	t.Helper()
	resp, data := DoBackendRequest(t, baseURL, method, path, body, user, backendAPIKey, signingSecret)
	defer resp.Body.Close()
	return resp.StatusCode, data
}

// BackendRawRequest sends a request with a raw body (not JSON-marshaled).
// Useful for tests that need to send invalid JSON or other raw payloads.
func BackendRawRequest(t *testing.T, baseURL, method, path string, raw []byte, user, backendAPIKey, signingSecret string) (int, []byte) {
	t.Helper()
	req, err := nethttp.NewRequest(method, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if user != "" {
		req.Header.Set("X-User-ID", user)
		req.Header.Set("X-User-Signature", SignHMAC(signingSecret, user))
	}
	if backendAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+backendAPIKey)
	}
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, bodyBytes
}

// CallSignUser makes a request to /v1/_sign with userId "u" and returns the JSON response map and status code.
func CallSignUser(t *testing.T, baseURL, backendAPIKey string) (map[string]string, int) {
	t.Helper()
	reqBody := []byte(`{"userId":"u"}`)
	req, err := nethttp.NewRequest("POST", baseURL+"/v1/_sign", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to build sign request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+backendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role-Name", "backend") // Explicitly set X-Role-Name as per instruction

	client := nethttp.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("sign request failed: %v", err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read sign response body: %v", err)
	}

	var out map[string]string
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		t.Fatalf("failed to unmarshal sign response: %v (body=%s)", err, string(bodyBytes))
	}
	return out, res.StatusCode
}
