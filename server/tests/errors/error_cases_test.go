package handlers

import (
	"bytes"
	"net/http"
	"testing"

	utils "progressdb/tests/utils"
)

func TestUnsigned_Call_To_Protected_Endpoint(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader([]byte(`{"body":{}}`)))
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode == 200 {
		t.Fatalf("expected error for unsigned request")
	}
}
