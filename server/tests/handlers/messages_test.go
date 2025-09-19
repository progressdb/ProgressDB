package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"

    utils "progressdb/tests/utils"
)

// Keep tests small and one-per-handler. No extra features here — simple, focused tests.

func TestCreateMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "msg_create"
    sig := utils.SignHMAC("signsecret", user)

    payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "hello"}}
    b, _ := json.Marshal(payload)
    req, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
    req.Header.Set("X-User-ID", user)
    req.Header.Set("X-User-Signature", sig)
    res, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("create request failed: %v", err)
    }
    if res.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", res.Status)
    }
    var out map[string]interface{}
    _ = json.NewDecoder(res.Body).Decode(&out)
    if id, _ := out["id"].(string); id == "" {
        t.Fatalf("missing id in response")
    }
}

func TestListMessages(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "msg_list"
    sig := utils.SignHMAC("signsecret", user)

    // create one message to list
    payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "listme"}}
    b, _ := json.Marshal(payload)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    thread := cout["thread"].(string)

    // list
    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/messages?thread="+thread, nil)
    lreq.Header.Set("X-User-ID", user)
    lreq.Header.Set("X-User-Signature", sig)
    lres, err := http.DefaultClient.Do(lreq)
    if err != nil {
        t.Fatalf("list request failed: %v", err)
    }
    if lres.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", lres.Status)
    }
    var listOut map[string]interface{}
    _ = json.NewDecoder(lres.Body).Decode(&listOut)
    if msgs, ok := listOut["messages"].([]interface{}); !ok || len(msgs) == 0 {
        t.Fatalf("expected messages in list result")
    }
}

func TestGetMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "msg_get"
    sig := utils.SignHMAC("signsecret", user)

    payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "gimme"}}
    b, _ := json.Marshal(payload)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    id := cout["id"].(string)

    greq, _ := http.NewRequest("GET", srv.URL+"/v1/messages/"+id, nil)
    greq.Header.Set("X-User-ID", user)
    greq.Header.Set("X-User-Signature", sig)
    gres, err := http.DefaultClient.Do(greq)
    if err != nil {
        t.Fatalf("get request failed: %v", err)
    }
    if gres.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", gres.Status)
    }
    var got map[string]interface{}
    _ = json.NewDecoder(gres.Body).Decode(&got)
    if gotID, _ := got["id"].(string); gotID != id {
        t.Fatalf("expected id %s got %s", id, gotID)
    }
}

func TestUpdateMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "msg_update"
    sig := utils.SignHMAC("signsecret", user)

    payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "old"}}
    b, _ := json.Marshal(payload)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    id := cout["id"].(string)

    // update
    time.Sleep(10 * time.Millisecond)
    up := map[string]interface{}{"author": user, "body": map[string]string{"text": "new"}}
    ub, _ := json.Marshal(up)
    ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/messages/"+id, bytes.NewReader(ub))
    ureq.Header.Set("X-User-ID", user)
    ureq.Header.Set("X-User-Signature", sig)
    ures, err := http.DefaultClient.Do(ureq)
    if err != nil {
        t.Fatalf("update request failed: %v", err)
    }
    if ures.StatusCode != 200 {
        t.Fatalf("expected 200 got %v", ures.Status)
    }
    var uout map[string]interface{}
    _ = json.NewDecoder(ures.Body).Decode(&uout)
    if body, ok := uout["body"].(map[string]interface{}); !ok || body["text"].(string) != "new" {
        t.Fatalf("expected updated body text")
    }
}

func TestDeleteMessage(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "msg_delete"
    sig := utils.SignHMAC("signsecret", user)

    payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "bye"}}
    b, _ := json.Marshal(payload)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    id := cout["id"].(string)
    thread := cout["thread"].(string)

    dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/messages/"+id, nil)
    dreq.Header.Set("X-User-ID", user)
    dreq.Header.Set("X-User-Signature", sig)
    dres, _ := http.DefaultClient.Do(dreq)
    if dres.StatusCode != 204 {
        t.Fatalf("delete failed: %v", dres.Status)
    }

    // ensure list no longer contains the message
    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/messages?thread="+thread, nil)
    lreq.Header.Set("X-User-ID", user)
    lreq.Header.Set("X-User-Signature", sig)
    lres, _ := http.DefaultClient.Do(lreq)
    var listOut map[string]interface{}
    _ = json.NewDecoder(lres.Body).Decode(&listOut)
    if msgs, ok := listOut["messages"].([]interface{}); ok {
        for _, m := range msgs {
            if mm, ok := m.(map[string]interface{}); ok {
                if mm["id"].(string) == id {
                    t.Fatalf("expected deleted message to be absent from list")
                }
            }
        }
    }
}

func TestListMessageVersions(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "msg_versions"
    sig := utils.SignHMAC("signsecret", user)

    payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "v1"}}
    b, _ := json.Marshal(payload)
    creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
    creq.Header.Set("X-User-ID", user)
    creq.Header.Set("X-User-Signature", sig)
    cres, _ := http.DefaultClient.Do(creq)
    var cout map[string]interface{}
    _ = json.NewDecoder(cres.Body).Decode(&cout)
    id := cout["id"].(string)

    // update once
    up := map[string]interface{}{"author": user, "body": map[string]string{"text": "v2"}}
    ub, _ := json.Marshal(up)
    ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/messages/"+id, bytes.NewReader(ub))
    ureq.Header.Set("X-User-ID", user)
    ureq.Header.Set("X-User-Signature", sig)
    http.DefaultClient.Do(ureq)

    // versions
    vreq, _ := http.NewRequest("GET", srv.URL+"/v1/messages/"+id+"/versions", nil)
    vreq.Header.Set("X-User-ID", user)
    vreq.Header.Set("X-User-Signature", sig)
    vres, _ := http.DefaultClient.Do(vreq)
    if vres.StatusCode != 200 {
        t.Fatalf("versions request failed: %v", vres.Status)
    }
    var vout map[string]interface{}
    _ = json.NewDecoder(vres.Body).Decode(&vout)
    if versions, ok := vout["versions"].([]interface{}); !ok || len(versions) < 2 {
        t.Fatalf("expected at least 2 versions")
    }
}
