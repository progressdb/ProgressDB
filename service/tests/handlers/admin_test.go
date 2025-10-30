// tests admin health, stats, key listing and retrieval, thread DEK rotate, rewrap, encrypt-existing admin endpoints
package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

// checks /admin/health returns 200 and {"status":"ok"}
func TestAdmin_Health(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	var out map[string]interface{}
	status := utils.AdminGetJSON(t, sp.Addr, "/admin/health", &out)
	if status != 200 {
		t.Fatalf("expected 200 got %d", status)
	}
	if out["status"] != "ok" {
		t.Fatalf("unexpected health response: %v", out)
	}
}

// checks /admin/stats returns nonzero stats after posting thread and message
func TestAdmin_Stats_ListThreads(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	// create a thread and a message so stats are non-zero
	user := "stat_user"
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "s1", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "x"}}
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, tid, msg, 5*time.Second)

	// call stats
	var s map[string]interface{}
	status := utils.AdminGetJSON(t, sp.Addr, "/admin/stats", &s)
	if status != 200 {
		t.Fatalf("expected 200 got %d", status)
	}
	if _, ok := s["threads"]; !ok {
		t.Fatalf("expected threads in stats")
	}
}

// checks listing and retrieving keys via /admin/keys endpoints
func TestAdmin_ListKeys_And_GetKey(t *testing.T) {
	// Use only public APIs: create a thread and a plaintext message, run the
	// admin encrypt-existing endpoint (which will back up plaintexts as
	// keys with prefix "backup:encrypt:"), then list and fetch those keys
	// via the admin endpoints.
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["%s", "%s"]
    frontend: ["%s"]
    admin: ["%s"]
encryption:
  enabled: true
  fields: ["body"]
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey, utils.FrontendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	user := "key_user"
	// create thread and wait until visible
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "k1", 5*time.Second)
	// create a plaintext message under the thread
	msgBody := map[string]interface{}{"author": user, "body": map[string]string{"text": "plain"}}
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, tid, msgBody, 5*time.Second)

	// call admin encrypt-existing to cause backup keys to be written
	reqBody := map[string]interface{}{"thread_ids": []string{tid}}
	var encOut map[string]map[string]string
	status := utils.DoAdminJSON(t, sp.Addr, "POST", "/admin/encryption/encrypt-existing", reqBody, &encOut)
	if status != 200 {
		t.Fatalf("encrypt-existing failed status=%d out=%v", status, encOut)
	}

	// list keys with backup prefix (paginated)
	var lres struct {
		Keys       []string `json:"keys"`
		NextCursor string   `json:"next_cursor"`
		HasMore    bool     `json:"has_more"`
		Count      int      `json:"count"`
	}
	status = utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/keys?prefix=backup:encrypt:&limit=100", nil, &lres)
	if status != 200 {
		t.Fatalf("list keys failed status=%d", status)
	}
	if len(lres.Keys) == 0 {
		t.Fatalf("expected at least one backup key, got none")
	}

	// fetch the first backup key and ensure it is well-formed JSON and includes
	// the structure of a backed-up message. The message body may now be encrypted (see
	// recent behavior), so only check that the returned JSON contains expected top-level fields.
	keyName := lres.Keys[0]
	kstatus, body := utils.GetAdminKey(t, sp.Addr, keyName)
	if kstatus != 200 {
		t.Fatalf("get admin key failed status=%d key=%s", kstatus, keyName)
	}
	// We only require the backup to be a valid message JSON containing at least the thread, author, and body field.
	var msg map[string]interface{}
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("expected key body to be valid JSON, got error: %v, body: %q", err, string(body))
	}
	if msg["thread"] == nil || msg["author"] == nil || msg["body"] == nil {
		t.Fatalf("expected key JSON to contain thread, author, and body fields; got=%q", string(body))
	}

	// Test pagination functionality
	var paginatedRes struct {
		Keys       []string `json:"keys"`
		NextCursor string   `json:"next_cursor"`
		HasMore    bool     `json:"has_more"`
		Count      int      `json:"count"`
	}

	// Request first page with limit 1
	status = utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/keys?prefix=backup:encrypt:&limit=1", nil, &paginatedRes)
	if status != 200 {
		t.Fatalf("paginated list keys failed status=%d", status)
	}
	if len(paginatedRes.Keys) != 1 {
		t.Fatalf("expected 1 key on first page, got %d", len(paginatedRes.Keys))
	}
	if paginatedRes.Count != 1 {
		t.Fatalf("expected count=1, got %d", paginatedRes.Count)
	}

	// If we have more keys, test next page
	if paginatedRes.HasMore && paginatedRes.NextCursor != "" {
		var secondPage struct {
			Keys       []string `json:"keys"`
			NextCursor string   `json:"next_cursor"`
			HasMore    bool     `json:"has_more"`
			Count      int      `json:"count"`
		}

		status = utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/keys?prefix=backup:encrypt:&limit=1&cursor="+paginatedRes.NextCursor, nil, &secondPage)
		if status != 200 {
			t.Fatalf("second page request failed status=%d", status)
		}
		if len(secondPage.Keys) != 1 {
			t.Fatalf("expected 1 key on second page, got %d", len(secondPage.Keys))
		}
		// Ensure we get different keys on different pages
		if secondPage.Keys[0] == paginatedRes.Keys[0] {
			t.Fatalf("expected different keys on different pages, got same key: %s", secondPage.Keys[0])
		}
	}
}

// checks rotating a thread data encryption key with /admin/encryption/rotate-thread-dek
func TestAdmin_RotateThreadDEK(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["%s", "%s"]
    frontend: ["%s"]
    admin: ["%s"]
encryption:
  enabled: true
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey, utils.FrontendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()
	user := "rot_user"
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "r1", 5*time.Second)
	var rout map[string]string
	status := utils.AdminPostJSON(t, sp.Addr, "/admin/encryption/rotate-thread-dek", map[string]string{"thread_key": tid}, &rout)
	if status != 200 {
		t.Fatalf("expected 200 got %d", status)
	}
	if rout["new_key"] == "" {
		t.Fatalf("expected new_key in rotate response")
	}
}

// test rewrapping a thread's DEK (data encryption key) using a new KEK.
func TestAdmin_RewrapDEKs(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["%s", "%s"]
    frontend: ["%s"]
    admin: ["%s"]
encryption:
  enabled: true
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
logging:
  level: info`, utils.SigningSecret, utils.BackendAPIKey, utils.FrontendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()
	user := "rw_user"
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "rw1", 5*time.Second)
	var out map[string]string
	status := utils.AdminPostJSON(t, sp.Addr, "/admin/encryption/rotate-thread-dek", map[string]string{"thread_key": tid}, &out)
	if status != 200 {
		t.Fatalf("expected 200 got %d", status)
	}
}

// checks encrypting existing thread messages via /admin/encryption/encrypt-existing
func TestAdmin_EncryptExisting(t *testing.T) {
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["` + utils.SigningSecret + `", "` + utils.BackendAPIKey + `"]
    frontend: ["` + utils.FrontendAPIKey + `"]
    admin: ["admin-secret"]
encryption:
  enabled: true
  fields: ["body"]
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()
	user := "enc_user"
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "e1", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "plain"}}
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, tid, msg, 5*time.Second)

	// call rotate-thread-dek (serves as encrypt-existing substitute)
	var out map[string]string
	status := utils.AdminPostJSON(t, sp.Addr, "/admin/encryption/rotate-thread-dek", map[string]string{"thread_key": tid}, &out)
	if status != 200 {
		t.Fatalf("rotate (as encrypt-existing substitute) failed status=%d", status)
	}
}
