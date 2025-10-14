package handlers

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

func logResponseBody(t *testing.T, body io.Reader, context string) {
	var out map[string]interface{}
	b, err := io.ReadAll(body)
	if err != nil {
		t.Logf("%s: failed to read body: %v", context, err)
		return
	}
	if len(b) == 0 {
		t.Logf("%s: response body is empty", context)
		return
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Logf("%s: failed to unmarshal body: %v, raw: %s", context, err, string(b))
		return
	}
	t.Logf("%s: response body=%v", context, out)
}

func TestCreateMessage(t *testing.T) {
	// Set up test server and user credentials
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "msg_create"

	// Prepare message payload
	payload := map[string]interface{}{"body": map[string]string{"text": "hello"}}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	t.Logf("TestCreateMessage: payload=%s", string(b))

	// Create a thread first, then POST the message under that thread using helpers
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "msg-thread", 5*time.Second)
	var out map[string]interface{}
	status := utils.FrontendPostJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", payload, user, &out)
	if status != 200 && status != 202 {
		t.Fatalf("expected 200 got %d", status)
	}
	if id, _ := out["id"].(string); id == "" {
		t.Fatalf("missing id in response")
	}
}

func TestListMessages(t *testing.T) {
	// Set up test server and user credentials
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "msg_list"

	// Create a thread and message, and wait until the message is visible.
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "listme"}}
	thID, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "list-thread", 5*time.Second)
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, thID, payload, 5*time.Second)

	// List messages in the thread
	var listOut map[string]interface{}
	status := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads/"+thID+"/messages", user, &listOut)
	if status != 200 {
		t.Fatalf("expected 200 got %d", status)
	}
	if msgs, ok := listOut["messages"].([]interface{}); !ok || len(msgs) == 0 {
		t.Fatalf("expected messages in list result")
	}
}
