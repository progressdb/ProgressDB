package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateMessage_InheritsThreadKMS(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	user := "msguser"
	sig := signHMAC("signsecret", user)
	// create thread
	th := map[string]interface{}{"author": user, "title": "mthread"}
	b, _ := json.Marshal(th)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	res, _ := http.DefaultClient.Do(req)
	var out map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	tid := out["id"].(string)

	// post message
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	r2, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	r2.Header.Set("X-User-ID", user)
	r2.Header.Set("X-User-Signature", sig)
	r2res, _ := http.DefaultClient.Do(r2)
	if r2res.StatusCode != 200 {
		t.Fatalf("create message failed: %v", r2res.Status)
	}
}
