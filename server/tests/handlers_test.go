package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

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
    admin: ["admin-secret"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// create a thread as admin (admin may supply author in body)
	thBody := map[string]string{"author": "alice", "title": "t1"}
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
	tid, ok := tout["id"].(string)
	if !ok || tid == "" {
		t.Fatalf("missing thread id in create response")
	}

	// create a message under thread as admin
	msg := map[string]interface{}{"author": "alice", "body": map[string]string{"text": "hello"}, "thread": tid}
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

	// list messages
	lreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+tid+"/messages", nil)
	lreq.Header.Set("Authorization", "Bearer admin-secret")
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	if lres.StatusCode != 200 {
		t.Fatalf("unexpected list messages status: %d", lres.StatusCode)
	}
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	_ = json.NewDecoder(lres.Body).Decode(&lout)
	if len(lout.Messages) == 0 {
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
    admin: ["admin-secret"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// invalid JSON to create thread -> 400
	req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Authorization", "Bearer admin-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid JSON; got %d", res.StatusCode)
	}

	// pagination: create 3 messages and request with limit=1
	thBody := map[string]string{"author": "alice", "title": "pg"}
	tb, _ := json.Marshal(thBody)
	creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(tb))
	creq.Header.Set("Authorization", "Bearer admin-secret")
	cres, _ := http.DefaultClient.Do(creq)
	var tout map[string]interface{}
	_ = json.NewDecoder(cres.Body).Decode(&tout)
	tid := tout["id"].(string)

	for i := 0; i < 3; i++ {
		m := map[string]interface{}{"author": "alice", "body": map[string]string{"text": "m"}, "thread": tid}
		mb, _ := json.Marshal(m)
		mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
		mreq.Header.Set("Authorization", "Bearer admin-secret")
		http.DefaultClient.Do(mreq)
	}

	lreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+tid+"/messages?limit=1", nil)
	lreq.Header.Set("Authorization", "Bearer admin-secret")
	lres, _ := http.DefaultClient.Do(lreq)
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	_ = json.NewDecoder(lres.Body).Decode(&lout)
	if len(lout.Messages) != 1 {
		t.Fatalf("expected 1 message due to limit; got %d", len(lout.Messages))
	}
}
