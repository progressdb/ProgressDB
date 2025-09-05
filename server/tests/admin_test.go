package api

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"

    "progressdb/pkg/store"
)

func TestAdminEndpoints(t *testing.T) {
    tmp := t.TempDir()
    dbdir := filepath.Join(tmp, "db")
    if err := store.Open(dbdir); err != nil {
        t.Fatalf("store.Open: %v", err)
    }
    srv := httptest.NewServer(Handler())
    defer srv.Close()
    client := srv.Client()

    // Admin health
    req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/health", nil)
    req.Header.Set("X-Role-Name", "admin")
    resp, err := client.Do(req)
    if err != nil { t.Fatalf("admin health: %v", err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp.StatusCode) }

    // Admin stats
    req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/stats", nil)
    req2.Header.Set("X-Role-Name", "admin")
    resp2, err := client.Do(req2)
    if err != nil { t.Fatalf("admin stats: %v", err) }
    defer resp2.Body.Close()
    var s struct{ Threads int `json:"threads"` }
    if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil { t.Fatalf("decode stats: %v", err) }

    // Admin threads (empty)
    req3, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/threads", nil)
    req3.Header.Set("X-Role-Name", "admin")
    resp3, err := client.Do(req3)
    if err != nil { t.Fatalf("admin threads: %v", err) }
    if resp3.StatusCode != http.StatusOK { t.Fatalf("expected 200, got %d", resp3.StatusCode) }
}

