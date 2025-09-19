package tests

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"progressdb/pkg/api"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
)

func setupServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	dbpath := dir + "/db"
	if err := logger.Init(); err != nil {
		t.Fatalf("logger.Init failed: %v", err)
	}
	if err := store.Open(dbpath); err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	prov, err := kms.NewHashicorpEmbeddedProvider(context.Background(), mk)
	if err != nil {
		t.Fatalf("NewHashicorpEmbeddedProvider: %v", err)
	}
	security.RegisterKMSProvider(prov)
	cfg := &config.RuntimeConfig{SigningKeys: map[string]struct{}{"signsecret": {}}}
	config.SetRuntime(cfg)
	return newServer(t, api.Handler())
}

func signHMAC(key, user string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(user))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestSign_Succeeds_For_Backend(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	body := map[string]string{"userId": "u1"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/_sign", bytes.NewReader(b))
	req.Header.Set("X-Role-Name", "backend")
	req.Header.Set("X-API-Key", "backend-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
}
