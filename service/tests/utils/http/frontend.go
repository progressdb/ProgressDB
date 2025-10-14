package httputil

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"testing"
)

// DoFrontendRequest performs an HTTP request using the provided frontend API key
// and signing secret. If user is non-empty, it will set X-User-ID and
// X-User-Signature signed with the signingSecret. Returns the response and
// body bytes.
func DoFrontendRequest(t *testing.T, baseURL, method, path string, body interface{}, user, frontendAPIKey, signingSecret string) (*nethttp.Response, []byte) {
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
	if frontendAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+frontendAPIKey)
	}
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return resp, bodyBytes
}

func FrontendGetJSON(t *testing.T, baseURL, path, user, frontendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	resp, data := DoFrontendRequest(t, baseURL, "GET", path, nil, user, frontendAPIKey, signingSecret)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

func FrontendPostJSON(t *testing.T, baseURL, path string, reqBody interface{}, user, frontendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	resp, data := DoFrontendRequest(t, baseURL, "POST", path, reqBody, user, frontendAPIKey, signingSecret)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

func FrontendPutJSON(t *testing.T, baseURL, path string, reqBody interface{}, user, frontendAPIKey, signingSecret string, out interface{}) int {
	t.Helper()
	resp, data := DoFrontendRequest(t, baseURL, "PUT", path, reqBody, user, frontendAPIKey, signingSecret)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

func FrontendRawRequest(t *testing.T, baseURL, method, path string, raw []byte, user, frontendAPIKey, signingSecret string) (int, []byte) {
	t.Helper()
	req, err := nethttp.NewRequest(method, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if user != "" {
		req.Header.Set("X-User-ID", user)
		req.Header.Set("X-User-Signature", SignHMAC(signingSecret, user))
	}
	if frontendAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+frontendAPIKey)
	}
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, bodyBytes
}
