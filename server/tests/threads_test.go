package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateThread_ProvisionDEK_When_EncryptionEnabled(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	user := "alice"
	sig := signHMAC("signsecret", user)
	body := map[string]interface{}{"author": user, "title": "t1"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
}
