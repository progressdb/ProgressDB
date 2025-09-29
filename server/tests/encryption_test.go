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
	"testing"

	"progressdb/pkg/kms"
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
logging:
  level: info
security:
  encryption:
    use: true
    fields: ["body"]
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
	var tout map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&tout)
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
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	_ = json.NewDecoder(lres.Body).Decode(&lout)
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
	if res.StatusCode != 200 && res.StatusCode != 201 {
		t.Fatalf("unexpected create thread status: %d", res.StatusCode)
	}
	var tout map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&tout)
	tid := tout["id"].(string)

	// get thread raw via admin list and inspect KMS metadata
	areq, _ := http.NewRequest("GET", sp.Addr+"/admin/threads", nil)
	areq.Header.Set("Authorization", "Bearer admin-secret")
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("admin threads request failed: %v", err)
	}
	var list struct {
		Threads []map[string]interface{} `json:"threads"`
	}
	_ = json.NewDecoder(ares.Body).Decode(&list)
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
