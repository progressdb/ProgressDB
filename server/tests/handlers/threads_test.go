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
	if _, err := http.DefaultClient.Do(creq); err != nil {
		t.Fatalf("create thread request failed: %v", err)
	}

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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := out["id"].(string)

	greq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid, nil)
	greq.Header.Set("X-User-ID", user)
	greq.Header.Set("X-User-Signature", sig)
	gres, err := http.DefaultClient.Do(greq)
	if err != nil {
		t.Fatalf("get thread failed: %v", err)
	}
	defer gres.Body.Close()
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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := out["id"].(string)

	up := map[string]interface{}{"title": "updated"}
	ub, _ := json.Marshal(up)
	ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid, bytes.NewReader(ub))
	ureq.Header.Set("X-User-ID", user)
	ureq.Header.Set("X-User-Signature", sig)
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Fatalf("update request failed: %v", err)
	}
	defer ures.Body.Close()
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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer res.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := out["id"].(string)

	dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/threads/"+tid, nil)
	dreq.Header.Set("X-User-ID", user)
	dreq.Header.Set("X-User-Signature", sig)
	dres, err := http.DefaultClient.Do(dreq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer dres.Body.Close()
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

	// create message in thread
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create thread message failed: %v", err)
	}
	defer mres.Body.Close()
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

	// create message
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	if _, err := http.DefaultClient.Do(mreq); err != nil {
		t.Fatalf("create message request failed: %v", err)
	}

	lreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages", nil)
	lreq.Header.Set("X-User-ID", user)
	lreq.Header.Set("X-User-Signature", sig)
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list thread messages failed: %v", err)
	}
	defer lres.Body.Close()
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

	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	defer mres.Body.Close()
	var mout map[string]interface{}
	if err := json.NewDecoder(mres.Body).Decode(&mout); err != nil {
		t.Fatalf("failed to decode create message response: %v", err)
	}
	mid := mout["id"].(string)

	greq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, nil)
	greq.Header.Set("X-User-ID", user)
	greq.Header.Set("X-User-Signature", sig)
	gres, err := http.DefaultClient.Do(greq)
	if err != nil {
		t.Fatalf("get thread message failed: %v", err)
	}
	defer gres.Body.Close()
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

	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	defer mres.Body.Close()
	var mout map[string]interface{}
	if err := json.NewDecoder(mres.Body).Decode(&mout); err != nil {
		t.Fatalf("failed to decode create message response: %v", err)
	}
	mid := mout["id"].(string)

	time.Sleep(10 * time.Millisecond)
	up := map[string]interface{}{"author": user, "body": map[string]string{"text": "updated"}}
	ub, _ := json.Marshal(up)
	ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, bytes.NewReader(ub))
	ureq.Header.Set("X-User-ID", user)
	ureq.Header.Set("X-User-Signature", sig)
	ures, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Fatalf("update request failed: %v", err)
	}
	defer ures.Body.Close()
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

	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mb, _ := json.Marshal(msg)
	mreq, _ := http.NewRequest("POST", srv.URL+"/v1/threads/"+tid+"/messages", bytes.NewReader(mb))
	mreq.Header.Set("X-User-ID", user)
	mreq.Header.Set("X-User-Signature", sig)
	mres, err := http.DefaultClient.Do(mreq)
	if err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	defer mres.Body.Close()
	var mout map[string]interface{}
	if err := json.NewDecoder(mres.Body).Decode(&mout); err != nil {
		t.Fatalf("failed to decode create message response: %v", err)
	}
	mid := mout["id"].(string)

	dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/threads/"+tid+"/messages/"+mid, nil)
	dreq.Header.Set("X-User-ID", user)
	dreq.Header.Set("X-User-Signature", sig)
	dres, err := http.DefaultClient.Do(dreq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	defer dres.Body.Close()
	if dres.StatusCode != 204 {
		t.Fatalf("delete thread message failed: %v", dres.Status)
	}
}
