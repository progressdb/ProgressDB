package tests

import (
	"fmt"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

// Tests end-to-end encryption and decryption of message fields.
func TestEncryption_E2E_EncryptRoundTrip(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["%s", "%s"]
    admin: ["%s"]
encryption:
  enabled: true
  fields: ["body"]
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create thread
	thBody := map[string]string{"author": "enc", "title": "enc-thread"}
	var tout map[string]interface{}
	status := utils.BackendPostJSON(t, sp.Addr, "/v1/threads", thBody, "enc", &tout)
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("create thread failed status=%d", status)
	}
	tid := tout["id"].(string)

	// create message
	msg := map[string]interface{}{"author": "enc", "body": map[string]string{"text": "secret"}, "thread": tid}
	mstatus := utils.BackendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", msg, "enc", nil)
	if mstatus != 200 && mstatus != 201 && mstatus != 202 {
		t.Fatalf("unexpected create message status: %d", mstatus)
	}

	time.Sleep((2 * time.Millisecond))

	// read back via API and verify plaintext
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	lstatus := utils.BackendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", "enc", &lout)
	if lstatus != 200 {
		t.Fatalf("list messages failed status=%d", lstatus)
	}
	if len(lout.Messages) == 0 {
		t.Fatalf("expected messages returned")
	}
	body := lout.Messages[0]["body"].(map[string]interface{})
	if body["text"] != "secret" {
		t.Fatalf("expected plaintext 'secret' got %#v", body["text"])
	}
}

// Tests DEK provisioning and KMS metadata on thread creation with encryption enabled.
func TestEncryption_E2E_ProvisionDEK(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["%s", "%s"]
    admin: ["%s"]
encryption:
  enabled: true
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create thread as backend - encryption should provision KMS meta
	thBody := map[string]string{"author": "enc", "title": "enc-thread"}
	var tout map[string]interface{}
	status := utils.BackendPostJSON(t, sp.Addr, "/v1/threads", thBody, "enc", &tout)
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("create thread failed status=%d", status)
	}
	tid := tout["id"].(string)

	// wait 2 seconds before calling to get the thread
	time.Sleep(2 * time.Millisecond)

	// get thread raw via admin list and inspect KMS metadata
	var list struct {
		Threads []map[string]interface{} `json:"threads"`
	}
	astatus := utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/threads", nil, &list)
	if astatus != 200 {
		t.Fatalf("admin threads request failed status=%d", astatus)
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
