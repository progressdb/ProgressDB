package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestAdmin_GenerateKEK(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/generate-kek", nil)
	req.Header.Set("X-Role-Name", "admin")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
	var out map[string]string
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out["kek_hex"] == "" {
		t.Fatalf("missing kek_hex")
	}
}
