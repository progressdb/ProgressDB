package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"

    utils "progressdb/tests/utils"
)

func TestCreateMessage_InheritsThreadKMS(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msguser"
	sig := utils.SignHMAC("signsecret", user)
	th := map[string]interface{}{"author": user, "title": "mthread"}
	b, _ := json.Marshal(th)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	res, _ := http.DefaultClient.Do(req)
	var out map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	tid := out["id"].(string)

	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}, "thread": tid}
	mb, _ := json.Marshal(msg)
	r2, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	r2.Header.Set("X-User-ID", user)
	r2.Header.Set("X-User-Signature", sig)
	r2res, _ := http.DefaultClient.Do(r2)
	if r2res.StatusCode != 200 {
		t.Fatalf("create message failed: %v", r2res.Status)
	}
}

func TestMessage_CRUD_And_Versions(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    user := "mcrud"
    sig := utils.SignHMAC("signsecret", user)

    th := map[string]interface{}{"author": user, "title": "t-m"}
    tb, _ := json.Marshal(th)
    treq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(tb))
    treq.Header.Set("X-User-ID", user)
    treq.Header.Set("X-User-Signature", sig)
    tres, _ := http.DefaultClient.Do(treq)
    var tout map[string]interface{}
    _ = json.NewDecoder(tres.Body).Decode(&tout)
    tid := tout["id"].(string)

    msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "first"}, "thread": tid}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", user)
    mreq.Header.Set("X-User-Signature", sig)
    mres, _ := http.DefaultClient.Do(mreq)
    if mres.StatusCode != 200 {
        t.Fatalf("create msg failed: %v", mres.Status)
    }
    var mout map[string]interface{}
    _ = json.NewDecoder(mres.Body).Decode(&mout)
    mid := mout["id"].(string)

    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
    lreq.Header.Set("X-User-ID", user)
    lreq.Header.Set("X-User-Signature", sig)
    lres, _ := http.DefaultClient.Do(lreq)
    var lob struct {
        Messages []map[string]interface{} `json:"messages"`
    }
    _ = json.NewDecoder(lres.Body).Decode(&lob)
    if len(lob.Messages) == 0 {
        t.Fatalf("expected messages")
    }

    time.Sleep(50 * time.Millisecond)

    up := map[string]interface{}{"body": map[string]string{"text": "second"}}
    ub, _ := json.Marshal(up)
    ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, bytes.NewReader(ub))
    ureq.Header.Set("X-User-ID", user)
    ureq.Header.Set("X-User-Signature", sig)
    ures, _ := http.DefaultClient.Do(ureq)
    if ures.StatusCode != 200 {
        t.Fatalf("update msg failed: %v", ures.Status)
    }

    // verify updated content appears in list
    lreq2, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
    lreq2.Header.Set("X-User-ID", user)
    lreq2.Header.Set("X-User-Signature", sig)
    lres2, _ := http.DefaultClient.Do(lreq2)
    var lob3 struct {
        Messages []map[string]interface{} `json:"messages"`
    }
    _ = json.NewDecoder(lres2.Body).Decode(&lob3)
    found := false
    for _, m := range lob3.Messages {
        if idv, ok := m["id"].(string); ok && idv == mid {
            if body, ok := m["body"].(map[string]interface{}); ok {
                if txt, ok := body["text"].(string); ok && txt == "second" {
                    found = true
                }
            }
        }
    }
    if !found {
        t.Fatalf("updated message not visible in list")
    }

    del := map[string]interface{}{"body": map[string]string{"text": "gone"}, "deleted": true}
    db, _ := json.Marshal(del)
    drew, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, bytes.NewReader(db))
    drew.Header.Set("X-User-ID", user)
    drew.Header.Set("X-User-Signature", sig)
    dres, _ := http.DefaultClient.Do(drew)
    if dres.StatusCode != 200 {
        t.Fatalf("mark deleted failed: %v", dres.Status)
    }

    l2, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
    l2.Header.Set("X-User-ID", user)
    l2.Header.Set("X-User-Signature", sig)
    lr2, _ := http.DefaultClient.Do(l2)
    var lob2 struct {
        Messages []map[string]interface{} `json:"messages"`
    }
    _ = json.NewDecoder(lr2.Body).Decode(&lob2)
    if len(lob2.Messages) != 0 {
        t.Fatalf("expected 0 messages after delete; got %d", len(lob2.Messages))
    }
}

// Test that listing messages by a different author is forbidden
func TestListMessages_AuthorAuthorization(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()
    a := "authorA"
    b := "authorB"
    sigA := utils.SignHMAC("signsecret", a)
    sigB := utils.SignHMAC("signsecret", b)
    th := map[string]interface{}{"author": a, "title": "auth"}
    tb, _ := json.Marshal(th)
    treq, _ := http.NewRequest("POST", srv.URL+"/v1/threads", bytes.NewReader(tb))
    treq.Header.Set("X-User-ID", a)
    treq.Header.Set("X-User-Signature", sigA)
    tres, _ := http.DefaultClient.Do(treq)
    var tout map[string]interface{}
    _ = json.NewDecoder(tres.Body).Decode(&tout)
    tid := tout["id"].(string)
    msg := map[string]interface{}{"author": a, "body": map[string]string{"text": "x"}, "thread": tid}
    mb, _ := json.Marshal(msg)
    mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
    mreq.Header.Set("X-User-ID", a)
    mreq.Header.Set("X-User-Signature", sigA)
    http.DefaultClient.Do(mreq)
    lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
    lreq.Header.Set("X-User-ID", b)
    lreq.Header.Set("X-User-Signature", sigB)
    lres, _ := http.DefaultClient.Do(lreq)
    if lres.StatusCode == 200 {
        t.Fatalf("expected forbidden for other author")
    }
}
