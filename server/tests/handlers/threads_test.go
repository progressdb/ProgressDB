package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"

    utils "progressdb/tests/utils"
)

// One focused test per handler in threads.go

func TestCreateThread(t *testing.T) {
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

func TestListThreads(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "list_alice"
    sig := utils.SignHMAC("signsecret", user)

    // create a thread then list
    body := map[string]interface{}{"author": user, "title": "lt1"}
    b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    http.DefaultClient.Do(creq)

    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads", nil)
    lreq.Header.Set("X-User-ID", user)
    lreq.Header.Set("X-User-Signature", sig)
    lres, err := http.DefaultClient.Do(lreq)
    if err != nil {
        t.Fatalf("list request failed: %v", err)
    }
    if lres.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", lres.Status)
    }
}

func TestGetThread(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "threaduser"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "orig"}
    b, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    req.Header.Set("X-User-ID", user)
    req.Header.Set("X-User-Signature", sig)
    res, _ := http.DefaultClient.Do(req)
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
}

func TestUpdateThread(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "threaduser"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "orig"}
    b, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    req.Header.Set("X-User-ID", user)
    req.Header.Set("X-User-Signature", sig)
    res, _ := http.DefaultClient.Do(req)
    var out map[string]interface{}
    _ = json.NewDecoder(res.Body).Decode(&out)
    tid := out["id"].(string)

    up := map[string]interface{}{"title": "updated"}
    ub, _ := json.Marshal(up)
    ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid, bytes.NewReader(ub))
    ureq.Header.Set("X-User-ID", user)
    ureq.Header.Set("X-User-Signature", sig)
    ures, _ := http.DefaultClient.Do(ureq)
    if ures.StatusCode != 200 {
        t.Fatalf("update failed: %v", ures.Status)
    }
}

func TestDeleteThread(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "threaduser"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "orig"}
    b, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    req.Header.Set("X-User-ID", user)
    req.Header.Set("X-User-Signature", sig)
    res, _ := http.DefaultClient.Do(req)
    var out map[string]interface{}
    _ = json.NewDecoder(res.Body).Decode(&out)
    tid := out["id"].(string)

    dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/threads/"+tid, nil)
    dreq.Header.Set("X-User-ID", user)
    dreq.Header.Set("X-User-Signature", sig)
    dres, _ := http.DefaultClient.Do(dreq)
    if dres.StatusCode != 204 {
        t.Fatalf("delete failed: %v", dres.Status)
    }
}

func TestCreateThreadMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "tm_user"
    sig := utils.SignHMAC("signsecret", user)

    // create thread
    body := map[string]interface{}{"author": user, "title": "tm"}
    b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    tid := cout["id"].(string)

    // create message in thread
    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    mres, _ := http.DefaultClient.Do(mreq)
    if mres.StatusCode != 200 {
        t.Fatalf("create thread message failed: %v", mres.Status)
    }
}

func TestListThreadMessages(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "tm_user"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "tm2"}
    b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    tid := cout["id"].(string)

    // create message
    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    http.DefaultClient.Do(mreq)

    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
    lreq.Header.Set("X-User-ID", user)
    lreq.Header.Set("X-User-Signature", sig)
    lres, _ := http.DefaultClient.Do(lreq)
    if lres.StatusCode != 200 {
        t.Fatalf("list thread messages failed: %v", lres.Status)
    }
}

func TestGetThreadMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "tm_user"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "tm3"}
    b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    tid := cout["id"].(string)

    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    mres, _ := http.DefaultClient.Do(mreq)
    var mout map[string]interface{}
    _ = json.NewDecoder(mres.Body).Decode(&mout)
    mid := mout["id"].(string)

    greq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, nil)
    greq.Header.Set("X-User-ID", user)
    greq.Header.Set("X-User-Signature", sig)
    gres, _ := http.DefaultClient.Do(greq)
    if gres.StatusCode != 200 {
        t.Fatalf("get thread message failed: %v", gres.Status)
    }
}

func TestUpdateThreadMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "tm_user"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "tm4"}
    b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    tid := cout["id"].(string)

    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    mres, _ := http.DefaultClient.Do(mreq)
    var mout map[string]interface{}
    _ = json.NewDecoder(mres.Body).Decode(&mout)
    mid := mout["id"].(string)

    time.Sleep(10 * time.Millisecond)
    up := map[string]interface{}{"author": user, "body": map[string]string{"text": "updated"}}
    ub, _ := json.Marshal(up)
    ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, bytes.NewReader(ub))
    ureq.Header.Set("X-User-ID", user)
    ureq.Header.Set("X-User-Signature", sig)
    ures, _ := http.DefaultClient.Do(ureq)
    if ures.StatusCode != 200 {
        t.Fatalf("update thread message failed: %v", ures.Status)
    }
}

func TestDeleteThreadMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "tm_user"
    sig := utils.SignHMAC("signsecret", user)

    body := map[string]interface{}{"author": user, "title": "tm5"}
    b, _ := json.Marshal(body)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    tid := cout["id"].(string)

    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    mres, _ := http.DefaultClient.Do(mreq)
    var mout map[string]interface{}
    _ = json.NewDecoder(mres.Body).Decode(&mout)
    mid := mout["id"].(string)

    dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, nil)
    dreq.Header.Set("X-User-ID", user)
    dreq.Header.Set("X-User-Signature", sig)
    dres, _ := http.DefaultClient.Do(dreq)
    if dres.StatusCode != 204 {
        t.Fatalf("delete thread message failed: %v", dres.Status)
    }
}
