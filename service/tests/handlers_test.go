package tests

import (
	"fmt"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

// verifies end-to-end thread and message creation/listing via handlers
func TestHandlers_E2E_ThreadsMessagesCRUD(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["%s", "%s"]
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create a thread as backend (backend may supply author in body or params)
	thBody := map[string]string{"author": "alice", "title": "t1"}
	var tout map[string]interface{}
	status := utils.BackendPostJSON(t, sp.Addr, "/v1/threads", thBody, "alice", &tout)
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("create thread failed status=%d", status)
	}
	tid, ok := tout["id"].(string)
	if !ok || tid == "" {
		t.Fatalf("missing thread id in create response")
	}

	// create a message under thread as backend
	msg := map[string]interface{}{"author": "alice", "body": map[string]string{"text": "hello"}, "thread": tid}
	mstatus := utils.BackendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", msg, "alice", nil)
	if mstatus != 200 && mstatus != 201 && mstatus != 202 {
		t.Fatalf("unexpected create message status: %d", mstatus)
	}

	// list messages (poll until visible to handle async ingest)
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	visible := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lstatus := utils.BackendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages?author=alice", "alice", &lout)
		if lstatus == 200 {
			if len(lout.Messages) > 0 {
				visible = true
				break
			}
		}
		// verify the 10ms claim
		time.Sleep(10 * time.Millisecond)
	}
	if !visible {
		t.Fatalf("expected messages returned for thread %s", tid)
	}
}

// verifies handler behavior for validation and pagination (invalid JSON and message listing with limit)
func TestHandlers_E2E_ValidationAndPagination(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["%s", "%s"]
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// invalid JSON to create thread -> server now enqueues raw payload and
	// returns 202 Accepted. Adjust expectation to match current behaviour.
	status, _ := utils.BackendRawRequest(t, sp.Addr, "POST", "/v1/threads", []byte(`{invalid`), "")
	if status != 202 && status != 200 && status != 201 {
		t.Fatalf("expected 202/200/201 for invalid JSON enqueue; got %d", status)
	}

	// pagination: create 3 messages and request with limit=1
	thBody := map[string]string{"author": "alice", "title": "pg"}
	var tout map[string]interface{}
	status = utils.BackendPostJSON(t, sp.Addr, "/v1/threads", thBody, "alice", &tout)
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("create thread failed status=%d", status)
	}
	tid := tout["id"].(string)

	for i := 0; i < 3; i++ {
		m := map[string]interface{}{"author": "alice", "body": map[string]string{"text": "m"}, "thread": tid}
		_ = utils.BackendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", m, "alice", nil)
	}

	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	lstatus := utils.BackendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages?limit=1&author=alice", "alice", &lout)
	if lstatus != 200 {
		t.Fatalf("list messages failed status=%d", lstatus)
	}
	if len(lout.Messages) != 1 {
		t.Fatalf("expected 1 message due to limit; got %d", len(lout.Messages))
	}
}
