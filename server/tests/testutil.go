package tests

import (
	"net"
	"net/http/httptest"
	"testing"
)

// newServer creates an httptest.Server bound to an IPv4 loopback listener.
// This returns a live server with srv.URL that can be used by http.Client.
func newServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen tcp4: %v", err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = ln
	srv.Start()
	return srv
}
