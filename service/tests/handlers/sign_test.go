package handlers

import (
	utils "progressdb/tests/utils"
	"testing"
)

func TestSign_Succeeds_For_Backend(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	body := map[string]string{"userId": "u1"}
	// Use backend helper which will add the backend API key and optional
	// signature headers.
	res, _ := utils.DoBackendRequest(t, sp.Addr, "POST", "/v1/_sign", body, "")
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
}
