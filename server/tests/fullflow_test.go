//go:build integration
// +build integration
package tests

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"

    utils "progressdb/tests/utils"
)

func TestE2E_ProvisionThenRotateThenRead(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "e2e"
    sig := utils.SignHMAC("signsecret", user)
    th := map[string]interface{}{"author": user, "title": "e2e-thread"}
    b, _ := json.Marshal(th)
    req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    req.Header.Set("X-User-ID", user)
    req.Header.Set("X-User-Signature", sig)
    res, _ := http.DefaultClient.Do(req)
    var out map[string]interface{}
    _ = json.NewDecoder(res.Body).Decode(&out)
    tid := out["id"].(string)
    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "before-rotate"}, "thread": tid}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    http.DefaultClient.Do(mreq)
    rreq := map[string]string{"thread_id": tid}
    rb, _ := json.Marshal(rreq)
    areq, _ := http.NewRequest("POST", srv.URL+"/admin/encryption/rotate-thread-dek", bytes.NewReader(rb))
    areq.Header.Set("X-Role-Name", "admin")
    ares, _ := http.DefaultClient.Do(areq)
    if ares.StatusCode != 200 {
        t.Fatalf("rotate failed: %v", ares.Status)
    }
    time.Sleep(100 * time.Millisecond)
    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
    lreq.Header.Set("X-User-ID", user)
    lreq.Header.Set("X-User-Signature", sig)
    lres, _ := http.DefaultClient.Do(lreq)
    var lob struct {
        Messages []map[string]interface{} `json:"messages"`
    }
    _ = json.NewDecoder(lres.Body).Decode(&lob)
    if len(lob.Messages) == 0 {
        t.Fatalf("expected messages after rotate")
    }
}

