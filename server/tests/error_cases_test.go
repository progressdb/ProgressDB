package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestUnsigned_Call_To_Protected_Endpoint(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	msg := map[string]interface{}{"author": "u", "body": map[string]string{"text": "x"}}
	b, _ := json.Marshal(msg)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %v", res.Status)
	}
}
