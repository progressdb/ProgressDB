package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	utils "progressdb/tests/utils"
	"testing"
)

func TestSign_Succeeds_For_Backend(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	body := map[string]string{"userId": "u1"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", sp.Addr+"/v1/_sign", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+utils.BackendAPIKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
}
