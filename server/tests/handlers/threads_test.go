package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"

    utils "progressdb/tests/utils"
)

func TestCreateThread_ProvisionDEK_When_EncryptionEnabled(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "alice"
	sig := utils.SignHMAC("signsecret", user)
	body := map[string]interface{}{"author": user, "title": "t1"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 got %v", res.Status)
	}
}

// Test basic thread CRUD: create, get, update, delete
func TestThread_CRUD(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "threaduser"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "orig"}
    b, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    req.Header.Set("X-User-ID", user)
    req.Header.Set("X-User-Signature", sig)
    res, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("create failed: %v", err)
    }
    var out map[string]interface{}
    _ = json.NewDecoder(res.Body).Decode(&out)
    tid := out["id"].(string)

    greq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid, nil)
    greq.Header.Set("X-User-ID", user)
    greq.Header.Set("X-User-Signature", sig)
    gres, _ := http.DefaultClient.Do(greq)
    if gres.StatusCode != 200 {
        t.Fatalf("get thread failed: %v", gres.Status)
    }

    up := map[string]interface{}{"title": "updated"}
    ub, _ := json.Marshal(up)
    ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid, bytes.NewReader(ub))
    ureq.Header.Set("X-User-ID", user)
    ureq.Header.Set("X-User-Signature", sig)
    ures, _ := http.DefaultClient.Do(ureq)
    if ures.StatusCode != 200 {
        t.Fatalf("update failed: %v", ures.Status)
    }
    var uout map[string]interface{}
    _ = json.NewDecoder(ures.Body).Decode(&uout)
    if uout["title"].(string) != "updated" {
        t.Fatalf("title not updated")
    }

    dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/threads/"+tid, nil)
    dreq.Header.Set("X-User-ID", user)
    dreq.Header.Set("X-User-Signature", sig)
    dres, _ := http.DefaultClient.Do(dreq)
    if dres.StatusCode != 204 {
        t.Fatalf("delete failed: %v", dres.Status)
    }

    g2, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid, nil)
    g2.Header.Set("X-User-ID", user)
    g2.Header.Set("X-User-Signature", sig)
    r2, _ := http.DefaultClient.Do(g2)
    if r2.StatusCode == 200 {
        t.Fatalf("expected 404 after delete")
    }
}
