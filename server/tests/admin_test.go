package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	api "progressdb/pkg/api"
	"progressdb/pkg/store"
)

func TestAdminEndpoints(t *testing.T) {
	tmp := t.TempDir()
	dbdir := filepath.Join(tmp, "db")
	if err := store.Open(dbdir); err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	h := api.Handler()

	// Admin health
	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	req.Header.Set("X-Role-Name", "admin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Admin stats
	req2 := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	req2.Header.Set("X-Role-Name", "admin")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("admin stats: got %d", rec2.Code)
	}
	var s struct {
		Threads int `json:"threads"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&s); err != nil {
		t.Fatalf("decode stats: %v", err)
	}

	// Admin threads (empty)
	req3 := httptest.NewRequest(http.MethodGet, "/admin/threads", nil)
	req3.Header.Set("X-Role-Name", "admin")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec3.Code)
	}
}
