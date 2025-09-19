package utils

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"progressdb/pkg/api"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
)

// localServer is a lightweight test server that routes requests directly to
// the provided handler without creating real network listeners. It replaces
// the global http.DefaultClient Transport while active and restores it on Close.
type LocalServer struct {
	URL  string
	prev *http.Client
}

func (s *LocalServer) Close() {
	if s.prev != nil {
		http.DefaultClient = s.prev
	}
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (h *handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	return rr.Result(), nil
}

// SetupServer initializes dependencies and returns a local test server that
// routes requests directly to the API handler.
func SetupServer(t *testing.T) *LocalServer {
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
	prev := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: &handlerRoundTripper{handler: api.Handler()}}
	return &LocalServer{URL: "http://localtest", prev: prev}
}

// SignHMAC returns hex HMAC-SHA256 of user using key
func SignHMAC(key, user string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(user))
	return hex.EncodeToString(mac.Sum(nil))
}
