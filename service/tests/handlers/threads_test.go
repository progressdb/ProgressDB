package handlers

import (
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

// One focused test per handler in threads.go

func TestCreateThread(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "alice"
	body := map[string]interface{}{"author": user, "title": "t1"}
	status := utils.FrontendPostJSON(t, sp.Addr, "/v1/threads", body, user, nil)
	if status != 200 && status != 202 {
		t.Fatalf("expected 200 or 202 got %d", status)
	}
}

func TestListThreads(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "list_alice"
	// create a thread then list
	body := map[string]interface{}{"author": user, "title": "lt1"}
	status := utils.FrontendPostJSON(t, sp.Addr, "/v1/threads", body, user, nil)
	if status != 200 && status != 201 && status != 202 {
		t.Fatalf("create thread request failed: %d", status)
	}

	var out map[string]interface{}
	lstatus := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads", user, &out)
	if lstatus != 200 {
		t.Fatalf("expected 200 got %d", lstatus)
	}
}

func TestGetThread(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "threaduser"
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "orig", 5*time.Second)
	var out map[string]interface{}
	gstatus := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads/"+tid, user, &out)
	if gstatus != 200 {
		t.Fatalf("get thread failed: %d", gstatus)
	}
}

func TestUpdateThread(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "threaduser"

	// create thread and update via frontend helper
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "orig", 5*time.Second)
	up := map[string]interface{}{"title": "updated"}
	status := utils.FrontendPutJSON(t, sp.Addr, "/v1/threads/"+tid, up, user, nil)
	if status != 200 && status != 202 {
		t.Fatalf("update failed: %d", status)
	}
}

func TestDeleteThread(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "threaduser"

	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "orig", 5*time.Second)
	status, _ := utils.FrontendRawRequest(t, sp.Addr, "DELETE", "/v1/threads/"+tid, nil, user)
	if status != 204 && status != 202 {
		t.Fatalf("delete failed: %d", status)
	}
}

func TestCreateThreadMessage(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "tm_user"

	// create thread and message via helpers
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "tm", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mstatus := utils.FrontendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", msg, user, nil)
	if mstatus != 200 && mstatus != 202 {
		t.Fatalf("create thread message failed: %d", mstatus)
	}
}

func TestListThreadMessages(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "tm_user"

	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "tm2", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, tid, msg, 5*time.Second)
	var lout struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	status := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", user, &lout)
	if status != 200 {
		t.Fatalf("list thread messages failed: %d", status)
	}
}

func TestGetThreadMessage(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "tm_user"

	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "tm4", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	var mout map[string]interface{}
	mstatus := utils.FrontendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", msg, user, &mout)
	if mstatus != 200 && mstatus != 201 && mstatus != 202 {
		t.Fatalf("create message failed: %d", mstatus)
	}
	mid := mout["id"].(string)
	// wait until message visible
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, tid, msg, 5*time.Second)

	var out map[string]interface{}
	gstatus := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages/"+mid, user, &out)
	if gstatus != 200 {
		t.Fatalf("get thread message failed: %d", gstatus)
	}
}

func TestUpdateThreadMessage(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "tm_user"

	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "tm4", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}
	mid := utils.CreateMessageAndWait(t, sp.Addr, user, tid, msg, 5*time.Second)

	// Wait for the created message to be visible before attempting the update.
	// Polling is preferred over a fixed small sleep to reduce flakiness on loaded
	// CI hosts.
	visible := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var out map[string]interface{}
		gstatus := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages/"+mid, user, &out)
		if gstatus == 200 {
			visible = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !visible {
		t.Fatalf("created message not visible for update after timeout")
	}
	up := map[string]interface{}{"author": user, "body": map[string]string{"text": "updated"}}
	status := utils.FrontendPutJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages/"+mid, up, user, nil)
	if status != 200 && status != 202 {
		t.Fatalf("update thread message failed: %d", status)
	}
}

func TestDeleteThreadMessage(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "tm_user"
	// create thread and message (no extra payload needed here)
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "tm5", 5*time.Second)
	var mout map[string]interface{}
	mstatus := utils.FrontendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", map[string]interface{}{"author": user, "body": map[string]string{"text": "hi"}}, user, &mout)
	if mstatus != 200 && mstatus != 201 && mstatus != 202 {
		t.Fatalf("create message failed: %d", mstatus)
	}
	mid := mout["id"].(string)
	status, _ := utils.FrontendRawRequest(t, sp.Addr, "DELETE", "/v1/threads/"+tid+"/messages/"+mid, nil, user)
	if status != 204 && status != 202 {
		t.Fatalf("delete thread message failed: %d", status)
	}
}
