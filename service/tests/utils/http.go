package utils

import (
	"net/http"
	"net/url"
	httpclient "progressdb/tests/utils/http"
	"strconv"
	"testing"
)

// Wrapper functions that preserve the original utils API surface while
// forwarding implementation to the cleaned `httputil` subpackage. The
// wrappers pass the package-level API keys and signing secret so callers
// don't need to change.

func DoBackendRequest(t *testing.T, baseURL, method, path string, body interface{}, user string) (*http.Response, []byte) {
	t.Helper()
	return httpclient.DoBackendRequest(t, baseURL, method, path, body, user, BackendAPIKey, SigningSecret)
}

func DoBackendJSON(t *testing.T, baseURL, method, path string, body interface{}, user string, out interface{}) int {
	t.Helper()
	return httpclient.DoBackendJSON(t, baseURL, method, path, body, user, BackendAPIKey, SigningSecret, out)
}

func BackendGetJSON(t *testing.T, baseURL, path, user string, out interface{}) int {
	t.Helper()
	return httpclient.BackendGetJSON(t, baseURL, path, user, BackendAPIKey, SigningSecret, out)
}

func BackendPostJSON(t *testing.T, baseURL, path string, req interface{}, user string, out interface{}) int {
	t.Helper()
	return httpclient.BackendPostJSON(t, baseURL, path, req, user, BackendAPIKey, SigningSecret, out)
}

func BackendPutJSON(t *testing.T, baseURL, path string, req interface{}, user string, out interface{}) int {
	t.Helper()
	return httpclient.BackendPutJSON(t, baseURL, path, req, user, BackendAPIKey, SigningSecret, out)
}

func BackendDeleteJSON(t *testing.T, baseURL, path string, user string, out interface{}) int {
	t.Helper()
	return httpclient.BackendDeleteJSON(t, baseURL, path, user, BackendAPIKey, SigningSecret, out)
}

func BackendRequest(t *testing.T, baseURL, method, path string, body interface{}, user string) (int, []byte) {
	t.Helper()
	return httpclient.BackendRequest(t, baseURL, method, path, body, user, BackendAPIKey, SigningSecret)
}

func DoFrontendRequest(t *testing.T, baseURL, method, path string, body interface{}, user string) (*http.Response, []byte) {
	t.Helper()
	return httpclient.DoFrontendRequest(t, baseURL, method, path, body, user, FrontendAPIKey, SigningSecret)
}

func FrontendGetJSON(t *testing.T, baseURL, path, user string, out interface{}) int {
	t.Helper()
	return httpclient.FrontendGetJSON(t, baseURL, path, user, FrontendAPIKey, SigningSecret, out)
}

func FrontendPostJSON(t *testing.T, baseURL, path string, reqBody interface{}, user string, out interface{}) int {
	t.Helper()
	return httpclient.FrontendPostJSON(t, baseURL, path, reqBody, user, FrontendAPIKey, SigningSecret, out)
}

func FrontendPutJSON(t *testing.T, baseURL, path string, reqBody interface{}, user string, out interface{}) int {
	t.Helper()
	return httpclient.FrontendPutJSON(t, baseURL, path, reqBody, user, FrontendAPIKey, SigningSecret, out)
}

func FrontendRawRequest(t *testing.T, baseURL, method, path string, raw []byte, user string) (int, []byte) {
	t.Helper()
	return httpclient.FrontendRawRequest(t, baseURL, method, path, raw, user, FrontendAPIKey, SigningSecret)
}

func DoAdminRequestRaw(t *testing.T, baseURL, method, path string, body interface{}) (*http.Response, []byte) {
	t.Helper()
	return httpclient.DoAdminRequestRaw(t, baseURL, method, path, body, AdminAPIKey)
}

func AdminGetJSON(t *testing.T, baseURL, path string, out interface{}) int {
	t.Helper()
	return httpclient.AdminGetJSON(t, baseURL, path, AdminAPIKey, out)
}

func AdminPostJSON(t *testing.T, baseURL, path string, reqBody interface{}, out interface{}) int {
	t.Helper()
	return httpclient.AdminPostJSON(t, baseURL, path, reqBody, AdminAPIKey, out)
}

func AdminGetKeyRaw(t *testing.T, baseURL, key string) (int, []byte) {
	t.Helper()
	return httpclient.AdminGetKeyRaw(t, baseURL, key, AdminAPIKey)
}

// High-level API helpers (preserve existing behavior)

func SignAsBackend(t *testing.T, baseURL, user string) (int, map[string]string) {
	t.Helper()
	var out map[string]string
	status := BackendPostJSON(t, baseURL, "/v1/_sign", map[string]string{"userId": user}, "", &out)
	return status, out
}

func CreateThreadAsBackend(t *testing.T, baseURL, author, title string) (int, map[string]interface{}) {
	t.Helper()
	body := map[string]string{"author": author, "title": title}
	var out map[string]interface{}
	status := BackendPostJSON(t, baseURL, "/v1/threads", body, author, &out)
	return status, out
}

func CreateMessageAsBackend(t *testing.T, baseURL, author, threadKey string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	if body == nil {
		body = map[string]interface{}{"author": author, "body": map[string]string{"text": "hello"}}
	}
	var out map[string]interface{}
	status := BackendPostJSON(t, baseURL, "/v1/threads/"+threadKey+"/messages", body, author, &out)
	return status, out
}

func ListThreadMessagesAsBackend(t *testing.T, baseURL, threadKey, author string, out interface{}) int {
	t.Helper()
	return BackendGetJSON(t, baseURL, "/v1/threads/"+threadKey+"/messages", author, out)
}

func AdminListKeysPaginated(t *testing.T, baseURL, prefix string, limit int, cursor string) (int, struct {
	Keys       []string `json:"keys"`
	NextCursor string   `json:"next_cursor"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}) {
	t.Helper()
	var out struct {
		Keys       []string `json:"keys"`
		NextCursor string   `json:"next_cursor"`
		HasMore    bool     `json:"has_more"`
		Count      int      `json:"count"`
	}

	requestURL := "/admin/keys?prefix=" + url.QueryEscape(prefix)
	if limit > 0 {
		requestURL += "&limit=" + strconv.Itoa(limit)
	}
	if cursor != "" {
		requestURL += "&cursor=" + url.QueryEscape(cursor)
	}

	status := AdminGetJSON(t, baseURL, requestURL, &out)
	return status, out
}

func AdminRotateThreadDEK(t *testing.T, baseURL, threadKey string) (int, map[string]string) {
	t.Helper()
	var out map[string]string
	status := AdminPostJSON(t, baseURL, "/admin/encryption/rotate-thread-dek", map[string]string{"thread_key": threadKey}, &out)
	return status, out
}

// BackendRawRequest sends a request with a raw body (not JSON-marshaled).
// Useful for tests that need to send invalid JSON or other raw payloads.
func BackendRawRequest(t *testing.T, baseURL, method, path string, raw []byte, user string) (int, []byte) {
	t.Helper()
	return httpclient.BackendRawRequest(t, baseURL, method, path, raw, user, BackendAPIKey, SigningSecret)
}
