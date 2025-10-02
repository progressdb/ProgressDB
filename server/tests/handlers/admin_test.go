package handlers

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "testing"

    "progressdb/pkg/store"

    utils "progressdb/tests/utils"
)

func TestAdmin_GenerateKEK(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/generate-kek", nil)
	req.Header.Set("X-Role-Name", "admin")
    res, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    defer res.Body.Close()
    if res.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", res.Status)
    }
    var out map[string]string
    if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
        t.Fatalf("failed to decode generate-kek response: %v", err)
    }
    if out["kek_hex"] == "" {
        t.Fatalf("missing kek_hex")
    }
}

func TestAdmin_Health(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/admin/health", nil)
	req.Header.Set("X-Role-Name", "admin")
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

func TestAdmin_Stats_ListThreads(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	// create a thread and a message so stats are non-zero
	user := "stat_user"
	sig := utils.SignHMAC("signsecret", user)
	body := map[string]interface{}{"author": user, "title": "s1"}
	b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
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
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "x"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
    if _, err := http.DefaultClient.Do(mreq); err != nil {
        t.Fatalf("create message failed: %v", err)
    }

	// call stats
	areq, _ := http.NewRequest("GET", srv.URL+"/admin/stats", nil)
	areq.Header.Set("X-Role-Name", "admin")
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

func TestAdmin_ListKeys_And_GetKey(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	// save a test key directly into store
	if err := store.SaveKey("testkey", []byte("val")); err != nil {
		t.Fatalf("store.SaveKey failed: %v", err)
	}

	// list keys
	lreq, _ := http.NewRequest("GET", srv.URL+"/admin/keys?prefix=test", nil)
	lreq.Header.Set("X-Role-Name", "admin")
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list keys failed: %v", err)
	}
	if lres.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", lres.Status)
	}

	// get specific key
    gres, err := http.DefaultClient.Do(func() *http.Request {
        req, _ := http.NewRequest("GET", srv.URL+"/admin/keys/testkey", nil)
        req.Header.Set("X-Role-Name", "admin")
        return req
    }())
    if err != nil {
        t.Fatalf("get key failed: %v", err)
    }
    defer gres.Body.Close()
    if gres.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", gres.Status)
    }
    // body should be raw bytes; read raw body and compare
    b, err := io.ReadAll(gres.Body)
    if err != nil {
        t.Fatalf("failed to read get key body: %v", err)
    }
    if string(b) != "val" {
        t.Fatalf("unexpected key body; want 'val' got %q", string(b))
    }
}

func TestAdmin_RotateThreadDEK(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "rot_user"
	sig := utils.SignHMAC("signsecret", user)
	body := map[string]interface{}{"author": user, "title": "r1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
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
	rreq, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/rotate-thread-dek", bytes.NewReader(rb))
	rreq.Header.Set("X-Role-Name", "admin")
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

func TestAdmin_RewrapDEKs(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "rw_user"
	sig := utils.SignHMAC("signsecret", user)
	body := map[string]interface{}{"author": user, "title": "rw1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
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

	// use a valid 64-hex kek (same value as testutil uses)
	mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	payload := map[string]interface{}{"thread_ids": []string{tid}, "new_kek_hex": mk, "parallelism": 1}
	pb, _ := json.Marshal(payload)
	rreq, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/rewrap-deks", bytes.NewReader(pb))
	rreq.Header.Set("X-Role-Name", "admin")
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil {
		t.Fatalf("rewrap request failed: %v", err)
	}
	if rres.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", rres.Status)
	}
}

func TestAdmin_EncryptExisting(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "enc_user"
	sig := utils.SignHMAC("signsecret", user)
	body := map[string]interface{}{"author": user, "title": "e1"}
	b, _ := json.Marshal(body)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
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

	// create a plaintext message
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "plain"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
    if _, err := http.DefaultClient.Do(mreq); err != nil {
        t.Fatalf("create message failed: %v", err)
    }

	payload := map[string]interface{}{"thread_ids": []string{tid}, "parallelism": 1}
	pb, _ := json.Marshal(payload)
	rreq, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/encrypt-existing", bytes.NewReader(pb))
	rreq.Header.Set("X-Role-Name", "admin")
	rres, err := http.DefaultClient.Do(rreq)
	if err != nil {
		t.Fatalf("encrypt-existing request failed: %v", err)
	}
	if rres.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", rres.Status)
	}
}
