package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

    "progressdb/pkg/auth"
    "progressdb/pkg/config"
)

func TestSignHandler_Success(t *testing.T) {
    // Start the API handler (sign route is registered on /v1/_sign)
    srv := httptest.NewServer(Handler())
    defer srv.Close()

	// Request payload
	payload := map[string]string{"userId": "u-test"}
	b, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/_sign", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	// sign handler expects caller to present backend role and the api key
    req.Header.Set("X-Role-Name", "backend")
    req.Header.Set("Authorization", "Bearer sk_test")

	// Call handler
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["userId"] != "u-test" {
		t.Fatalf("unexpected userId: %v", out)
	}
	// verify signature matches HMAC of provided key
	mac := hmac.New(sha256.New, []byte("sk_test"))
	mac.Write([]byte("u-test"))
	expected := hex.EncodeToString(mac.Sum(nil))
	if out["signature"] != expected {
		t.Fatalf("signature mismatch; expected %s got %s", expected, out["signature"])
	}
}

func TestRequireSignedAuthorMiddleware(t *testing.T) {
	// configure runtime signing keys
	rc := &config.RuntimeConfig{SigningKeys: map[string]struct{}{"sk_test": {}}, BackendKeys: map[string]struct{}{}}
	config.SetRuntime(rc)

	// protected handler: echoes the verified author id
	h := auth.RequireSignedAuthor(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, auth.AuthorIDFromContext(r.Context()))
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	// create valid signature
	mac := hmac.New(sha256.New, []byte("sk_test"))
	mac.Write([]byte("user-1"))
	sig := hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("X-User-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 got %d: %s", resp.StatusCode, string(b))
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "user-1" {
		t.Fatalf("unexpected body: %q", string(body))
	}

	// now try invalid signature
	req2, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req2.Header.Set("X-User-ID", "user-1")
	req2.Header.Set("X-User-Signature", "bad")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 401 got %d: %s", resp2.StatusCode, string(b))
	}
}
