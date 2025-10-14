package httputil

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/url"
	"testing"
)

// DoAdminRequestRaw performs an HTTP request using the provided admin API key.
// Returns response and body bytes.
func DoAdminRequestRaw(t *testing.T, baseURL, method, path string, body interface{}, adminAPIKey string) (*nethttp.Response, []byte) {
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
	if adminAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+adminAPIKey)
	}
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return resp, bodyBytes
}

func AdminGetJSON(t *testing.T, baseURL, path, adminAPIKey string, out interface{}) int {
	t.Helper()
	resp, data := DoAdminRequestRaw(t, baseURL, "GET", path, nil, adminAPIKey)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

func AdminPostJSON(t *testing.T, baseURL, path string, reqBody interface{}, adminAPIKey string, out interface{}) int {
	t.Helper()
	resp, data := DoAdminRequestRaw(t, baseURL, "POST", path, reqBody, adminAPIKey)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

func AdminGetKeyRaw(t *testing.T, baseURL, key, adminAPIKey string) (int, []byte) {
	t.Helper()
	esc := url.PathEscape(key)
	resp, data := DoAdminRequestRaw(t, baseURL, "GET", "/admin/keys/"+esc, nil, adminAPIKey)
	defer resp.Body.Close()
	return resp.StatusCode, data
}
