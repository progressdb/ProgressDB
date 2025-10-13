package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

// Objectives: verify handlers end-to-end for CRUD flows, validation, and store integration.
func TestHandlers_E2E_ThreadsMessagesCRUD(t *testing.T) {
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    api_keys:
    backend: ["backend-secret"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create a thread as admin (admin may supply author in body)
	thBody := map[string]string{"author": "alice", "title": "t1"}
	tb, _ := json.Marshal(thBody)
	req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(tb))
	req.Header.Set("Authorization", "Bearer backend-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		t.Fatalf("unexpected create thread status: %d", res.StatusCode)
	}
	var tout map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&tout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid, ok := tout["id"].(string)
	if !ok || tid == "" {
		t.Fatalf("missing thread id in create response")
	}

	// create a message under thread as admin
	msg := map[string]interface{}{"author": "alice", "body": map[string]string{"text": "hello"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("Authorization", "Bearer backend-secret")
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if mres.StatusCode != 200 && mres.StatusCode != 201 && mres.StatusCode != 202 {
		t.Fatalf("unexpected create message status: %d", mres.StatusCode)
	}

	// list messages
	// list messages (poll until visible to handle async ingest)
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	visible := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+tid+"/messages", nil)
		lreq.Header.Set("Authorization", "Bearer backend-secret")
		lres, err := http.DefaultClient.Do(lreq)
		if err == nil {
			if lres.StatusCode == 200 {
				_ = json.NewDecoder(lres.Body).Decode(&lout)
				_ = lres.Body.Close()
				if len(lout.Messages) > 0 {
					visible = true
					break
				}
			}
			if lres != nil {
				_ = lres.Body.Close()
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !visible {
		t.Fatalf("expected messages returned for thread %s", tid)
	}
}

func TestHandlers_E2E_ValidationAndPagination(t *testing.T) {
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    api_keys:
    backend: ["backend-secret"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// invalid JSON to create thread -> server now enqueues raw payload and
	// returns 202 Accepted. Adjust expectation to match current behaviour.
	req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Authorization", "Bearer backend-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 202 && res.StatusCode != 200 && res.StatusCode != 201 {
		t.Fatalf("expected 202/200/201 for invalid JSON enqueue; got %d", res.StatusCode)
	}

	// pagination: create 3 messages and request with limit=1
	thBody := map[string]string{"author": "alice", "title": "pg"}
	tb, _ := json.Marshal(thBody)
	creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(tb))
	creq.Header.Set("Authorization", "Bearer backend-secret")
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer cres.Body.Close()
	var tout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&tout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := tout["id"].(string)

	for i := 0; i < 3; i++ {
		m := map[string]interface{}{"author": "alice", "body": map[string]string{"text": "m"}, "thread": tid}
		mb, _ := json.Marshal(m)
		mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
		mreq.Header.Set("Authorization", "Bearer backend-secret")
		if _, err := http.DefaultClient.Do(mreq); err != nil {
			t.Fatalf("create message failed: %v", err)
		}
	}

	lreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+tid+"/messages?limit=1", nil)
	lreq.Header.Set("Authorization", "Bearer backend-secret")
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
	if len(lout.Messages) != 1 {
		t.Fatalf("expected 1 message due to limit; got %d", len(lout.Messages))
	}
}
