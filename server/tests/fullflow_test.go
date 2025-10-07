//go:build integration
// +build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

func TestE2E_ProvisionThenRotateThenRead(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "e2e"
	sig := utils.SignHMAC(utils.SigningSecret, user)
	th := map[string]interface{}{"author": user, "title": "e2e-thread"}
	b, _ := json.Marshal(th)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	req.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := out["id"].(string)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "before-rotate"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	if _, err := http.DefaultClient.Do(mreq); err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	rreq := map[string]string{"thread_id": tid}
	rb, _ := json.Marshal(rreq)
	areq, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/rotate-thread-dek", bytes.NewReader(rb))
	areq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("rotate request failed: %v", err)
	}
	defer ares.Body.Close()
	if ares.StatusCode != 200 {
		t.Fatalf("rotate failed: %v", ares.Status)
	}
	time.Sleep(100 * time.Millisecond)
	lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
	lreq.Header.Set("X-User-ID", user)
	lreq.Header.Set("X-User-Signature", sig)
	lreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	defer lres.Body.Close()
	var lob struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.NewDecoder(lres.Body).Decode(&lob); err != nil {
		t.Fatalf("failed to decode list messages response: %v", err)
	}
	if len(lob.Messages) == 0 {
		t.Fatalf("expected messages after rotate")
	}
}
