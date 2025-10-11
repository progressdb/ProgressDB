package utils

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api"
	"progressdb/pkg/auth"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/store"
)

const (
	BackendAPIKey  = "backend-test"
	FrontendAPIKey = "frontend-test"
	AdminAPIKey    = "admin-test"
	SigningSecret  = "signsecret"
)

// LocalServer starts an in-process fasthttp server bound to an ephemeral
// localhost port. Tests can send regular net/http requests to LocalServer.URL.
type LocalServer struct {
	URL string

	srv *fasthttp.Server
	ln  net.Listener
}

func (s *LocalServer) Close() {
	if s.srv != nil {
		_ = s.srv.Shutdown()
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
	logger.Sync()
}

// SetupServer initializes dependencies and starts a fasthttp server listening
// on an ephemeral localhost port for use in tests.
func SetupServer(t *testing.T) *LocalServer {
	t.Helper()

	workdir := NewArtifactsDir(t, "inproc-server")
	dbpath := filepath.Join(workdir, "db")
	storePath := filepath.Join(dbpath, "store")
	_ = os.MkdirAll(storePath, 0o700)

	logger.Init()
	if err := store.Open(storePath, true); err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}

	mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	prov, err := kms.NewHashicorpEmbeddedProvider(context.Background(), mk)
	if err != nil {
		t.Fatalf("NewHashicorpEmbeddedProvider: %v", err)
	}
	kms.RegisterKMSProvider(prov)

	cfg := &config.RuntimeConfig{
		BackendKeys: map[string]struct{}{
			BackendAPIKey:  {},
			FrontendAPIKey: {},
			AdminAPIKey:    {},
		},
		SigningKeys: map[string]struct{}{SigningSecret: {}},
	}
	config.SetRuntime(cfg)

	secCfg := auth.SecConfig{
		BackendKeys:  map[string]struct{}{BackendAPIKey: {}},
		FrontendKeys: map[string]struct{}{FrontendAPIKey: {}},
		AdminKeys:    map[string]struct{}{AdminAPIKey: {}},
	}
	h := api.Handler()
	wrapped := auth.RequireSignedAuthorFast(h)
	wrapped = auth.AuthenticateRequestMiddlewareFast(secCfg)(wrapped)

	srv := &fasthttp.Server{Handler: wrapped}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		if err := srv.Serve(ln); err != nil {
			t.Logf("fasthttp server stopped: %v", err)
		}
	}()

	// Give the server a brief moment to start accepting connections.
	time.Sleep(10 * time.Millisecond)

	return &LocalServer{URL: "http://" + ln.Addr().String(), srv: srv, ln: ln}
}

// SignHMAC returns hex HMAC-SHA256 of user using key.
func SignHMAC(key, user string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(user))
	return hex.EncodeToString(mac.Sum(nil))
}
