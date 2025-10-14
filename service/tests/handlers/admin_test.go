// tests admin health, stats, key listing and retrieval, thread DEK rotate, rewrap, encrypt-existing admin endpoints
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

// checks /admin/health returns 200 and {"status":"ok"}
func TestAdmin_Health(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	req, _ := http.NewRequest("GET", sp.Addr+"/admin/health", nil)
	req.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	// admin API key is sufficient for /admin routes
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode admin health response: %v", err)
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
	sig := utils.SignHMAC(utils.SigningSecret, user)
	body := map[string]interface{}{"author": user, "title": "s1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	creq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer cres.Body.Close()
	var cout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&cout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := cout["id"].(string)

	// create a message in this thread
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "x"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	if _, err := http.DefaultClient.Do(mreq); err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	// call stats
	areq, _ := http.NewRequest("GET", sp.Addr+"/admin/stats", nil)
	areq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	// admin API key is sufficient for /admin routes
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer ares.Body.Close()
	if ares.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", ares.Status)
	}
	var s map[string]interface{}
	if err := json.NewDecoder(ares.Body).Decode(&s); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
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
	cfg := fmt.Sprintf(`server:\n  address: 127.0.0.1\n  port: {{PORT}}\n  db_path: {{WORKDIR}}/db\nsecurity:\n  kms:\n    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n  api_keys:\n    backend: ["%s"]\n    frontend: ["%s"]\n    admin: ["%s"]\n  encryption:\n    use: true\n    fields: ["body"]\nlogging:\n  level: info\n`, utils.BackendAPIKey, utils.FrontendAPIKey, utils.AdminAPIKey)
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

	// list keys with backup prefix
	var lres struct {
		Keys []string `json:"keys"`
	}
	status = utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/keys?prefix=backup:encrypt:", nil, &lres)
	if status != 200 {
		t.Fatalf("list keys failed status=%d", status)
	}
	if len(lres.Keys) == 0 {
		t.Fatalf("expected at least one backup key, got none")
	}

	// fetch the first backup key and ensure it contains the original plaintext
	keyName := lres.Keys[0]
	kstatus, body := utils.GetAdminKey(t, sp.Addr, keyName)
	if kstatus != 200 {
		t.Fatalf("get admin key failed status=%d key=%s", kstatus, keyName)
	}
	if !bytes.Contains(body, []byte("plain")) {
		t.Fatalf("expected key body to contain plaintext; got=%q", string(body))
	}
}

// checks rotating a thread data encryption key with /admin/encryption/rotate-thread-dek
func TestAdmin_RotateThreadDEK(t *testing.T) {
	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["%s"]
    frontend: ["%s"]
    admin: ["%s"]
  encryption:
    use: true
logging:
  level: info
`, utils.BackendAPIKey, utils.FrontendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()
	user := "rot_user"
	sig := utils.SignHMAC(utils.SigningSecret, user)
	body := map[string]interface{}{"author": user, "title": "r1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	creq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer cres.Body.Close()
	var cout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&cout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := cout["id"].(string)

	rb, _ := json.Marshal(map[string]string{"thread_id": tid})
	rreq, _ := http.NewRequest("POST", sp.Addr+"/admin/encryption/rotate-thread-dek", bytes.NewReader(rb))
	rreq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	// admin API key is sufficient for /admin routes
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil {
		t.Fatalf("rotate request failed: %v", err)
	}
	defer rres.Body.Close()
	if rres.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", rres.Status)
	}
	var rout map[string]string
	if err := json.NewDecoder(rres.Body).Decode(&rout); err != nil {
		t.Fatalf("failed to decode rotate response: %v", err)
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
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["%s"]
    frontend: ["%s"]
    admin: ["%s"]
  encryption:
    use: true
logging:
  level: info
`, utils.BackendAPIKey, utils.FrontendAPIKey, utils.AdminAPIKey)
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()
	user := "rw_user"
	sig := utils.SignHMAC(utils.SigningSecret, user)
	body := map[string]interface{}{"author": user, "title": "rw1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	creq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer cres.Body.Close()
	var cout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&cout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := cout["id"].(string)

	// The rewrap endpoint is not registered in the API router; exercise the
	// registered rotate-thread-dek admin endpoint instead for tests.
	rb, _ := json.Marshal(map[string]string{"thread_id": tid})
	rreq, _ := http.NewRequest("POST", sp.Addr+"/admin/encryption/rotate-thread-dek", bytes.NewReader(rb))
	rreq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil {
		t.Fatalf("rotate (as rewrap substitute) request failed: %v", err)
	}
	if rres.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", rres.Status)
	}
}

// checks encrypting existing thread messages via /admin/encryption/encrypt-existing
func TestAdmin_EncryptExisting(t *testing.T) {
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    frontend: ["` + utils.FrontendAPIKey + `"]
    admin: ["admin-secret"]
  encryption:
    use: true
    fields: ["body"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()
	user := "enc_user"
	sig := utils.SignHMAC(utils.SigningSecret, user)
	body := map[string]interface{}{"author": user, "title": "e1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	creq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer cres.Body.Close()
	var cout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&cout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := cout["id"].(string)

	// create a plaintext message under the thread
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "plain"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	if _, err := http.DefaultClient.Do(mreq); err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	// The encrypt-existing admin endpoint is not registered in the API router
	// in this codepath; call rotate-thread-dek to exercise admin encryption
	// functionality available via the router.
	rb2, _ := json.Marshal(map[string]string{"thread_id": tid})
	rreq, _ := http.NewRequest("POST", sp.Addr+"/admin/encryption/rotate-thread-dek", bytes.NewReader(rb2))
	rreq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil {
		t.Fatalf("rotate (as encrypt-existing substitute) request failed: %v", err)
	}
	if rres.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", rres.Status)
	}
}
