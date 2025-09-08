package api_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	api "progressdb/pkg/api"
	"progressdb/pkg/auth"
	"progressdb/pkg/config"
)

func TestSignHandler_Success(t *testing.T) {
	h := api.Handler()

	payload := map[string]string{"userId": "u-test"}
	b, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/v1/_sign", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role-Name", "backend")
	req.Header.Set("Authorization", "Bearer sk_test")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["userId"] != "u-test" {
		t.Fatalf("unexpected userId: %v", out)
	}

	mac := hmac.New(sha256.New, []byte("sk_test"))
	mac.Write([]byte("u-test"))
	expected := hex.EncodeToString(mac.Sum(nil))
	if out["signature"] != expected {
		t.Fatalf("signature mismatch; expected %s got %s", expected, out["signature"])
	}
}

func TestRequireSignedAuthorMiddleware(t *testing.T) {
	rc := &config.RuntimeConfig{SigningKeys: map[string]struct{}{"sk_test": {}}, BackendKeys: map[string]struct{}{}}
	config.SetRuntime(rc)

	h := auth.RequireSignedAuthor(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, auth.AuthorIDFromContext(r.Context()))
	}))
	mac := hmac.New(sha256.New, []byte("sk_test"))
	mac.Write([]byte("user-1"))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("X-User-Signature", sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "user-1" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-User-ID", "user-1")
	req2.Header.Set("X-User-Signature", "bad")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d: %s", rec2.Code, rec2.Body.String())
	}
}
