//go:build integration
// +build integration

package tests

// Objectives (from docs/tests.md):
// 1. Verify DEK provisioning on thread creation when encryption is enabled.
// 2. Verify encryption/decryption round-trips: API returns plaintext while DB stores ciphertext.
// 3. Verify DEK rotation keeps messages decryptable.
// 4. Verify KMS/mk validation and fail-fast behavior for missing master keys.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"progressdb/pkg/kms"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	utils "progressdb/tests/utils"
)

// The substantive E2E encryption tests are implemented as standalone
// tests below: TestEncryption_E2E_EncryptRoundTrip and
// TestEncryption_E2E_ProvisionDEK. The older suite wrapper has been
// removed to avoid duplication.

func TestEncryption_E2E_EncryptRoundTrip(t *testing.T) {
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    admin: ["admin-secret"]
  encryption:
    use: true
    fields: ["body"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create thread
	thBody := map[string]string{"author": "enc", "title": "enc-thread"}
	tb, _ := json.Marshal(thBody)
	req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(tb))
	req.Header.Set("Authorization", "Bearer admin-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	var tout map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&tout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := tout["id"].(string)

	// create message
	msg := map[string]interface{}{"author": "enc", "body": map[string]string{"text": "secret"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("Authorization", "Bearer admin-secret")
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	defer mres.Body.Close()
	if mres.StatusCode != 200 && mres.StatusCode != 201 {
		t.Fatalf("unexpected create message status: %d", mres.StatusCode)
	}

	// read back via API and verify plaintext
	lreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+tid+"/messages", nil)
	lreq.Header.Set("Authorization", "Bearer admin-secret")
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	defer lres.Body.Close()
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.NewDecoder(lres.Body).Decode(&lout); err != nil {
		t.Fatalf("failed to decode list messages response: %v", err)
	}
	if len(lout.Messages) == 0 {
		t.Fatalf("expected messages returned")
	}
	body := lout.Messages[0]["body"].(map[string]interface{})
	if body["text"] != "secret" {
		t.Fatalf("expected plaintext 'secret' got %#v", body["text"])
	}
}

func TestEncryption_E2E_ProvisionDEK(t *testing.T) {
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    admin: ["admin-secret"]
logging:
  level: info
  `
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create thread as admin - encryption should provision KMS meta
	thBody := map[string]string{"author": "enc", "title": "enc-thread"}
	tb, _ := json.Marshal(thBody)
	req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(tb))
	req.Header.Set("Authorization", "Bearer admin-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 201 {
		t.Fatalf("unexpected create thread status: %d", res.StatusCode)
	}
	var tout map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&tout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := tout["id"].(string)

	// get thread raw via admin list and inspect KMS metadata
	areq, _ := http.NewRequest("GET", sp.Addr+"/admin/threads", nil)
	areq.Header.Set("Authorization", "Bearer admin-secret")
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("admin threads request failed: %v", err)
	}
	defer ares.Body.Close()
	var list struct {
		Threads []map[string]interface{} `json:"threads"`
	}
	if err := json.NewDecoder(ares.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode admin threads response: %v", err)
	}
	found := false
	for _, titem := range list.Threads {
		if titem["id"] == tid {
			if _, ok := titem["kms"]; ok {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected KMS metadata for thread %s", tid)
	}
}

// In-process test: verifies that encrypted messages are stored with the
// encryption marker ("_enc") while the API returns plaintext. This uses
// utils.SetupServer (in-process handler + store) so the test can inspect
// raw stored keys via store.ListKeys/GetKey.
func TestEncryption_InProcess_StoredCiphertext(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()

	// enable field-level encryption for the body in this process
	if err := security.SetEncryptionFieldPolicy([]string{"body"}); err != nil {
		t.Fatalf("SetEncryptionFieldPolicy failed: %v", err)
	}

	user := "enc_user"
	sig := utils.SignHMAC("signsecret", user)

	// create thread
	thBody := map[string]string{"author": user, "title": "enc-thread"}
	tb, _ := json.Marshal(thBody)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(tb))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	var tout map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&tout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := tout["id"].(string)

	// create message
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "secret"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	defer mres.Body.Close()
	if mres.StatusCode != 200 && mres.StatusCode != 201 {
		t.Fatalf("unexpected create message status: %d", mres.StatusCode)
	}

	// read back via API and verify plaintext
	lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
	lreq.Header.Set("X-User-ID", user)
	lreq.Header.Set("X-User-Signature", sig)
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	defer lres.Body.Close()
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.NewDecoder(lres.Body).Decode(&lout); err != nil {
		t.Fatalf("failed to decode list messages response: %v", err)
	}
	if len(lout.Messages) == 0 {
		t.Fatalf("expected messages returned")
	}
	body := lout.Messages[0]["body"].(map[string]interface{})
	if body["text"] != "secret" {
		t.Fatalf("expected plaintext 'secret' got %#v", body["text"])
	}

	// Inspect raw stored value in Pebble to ensure it contains the encryption marker
	// Message keys are prefixed with "thread:<tid>:msg:" — list keys and fetch one.
	mp, _ := store.MsgPrefix(tid)
	keys, err := store.ListKeys(mp)
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) == 0 {
		t.Fatalf("expected message keys in store for thread %s", tid)
	}
	raw, err := store.GetKey(keys[0])
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}
	if !strings.Contains(raw, "\"_enc\"") {
		t.Fatalf("expected stored message to contain encryption marker _enc; got: %s", raw)
	}
}
