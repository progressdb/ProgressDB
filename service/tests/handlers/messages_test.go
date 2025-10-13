package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	utils "progressdb/tests/utils"
	"testing"
	"time"
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
	sig := utils.SignHMAC(utils.SigningSecret, user)

	t.Logf("TestCreateMessage: user=%s, sig=%s", user, sig)

	// Prepare message payload
	payload := map[string]interface{}{"body": map[string]string{"text": "hello"}}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	t.Logf("TestCreateMessage: payload=%s", string(b))

	// Create a thread first, then POST the message under that thread
	thBody := map[string]string{"author": user, "title": "msg-thread"}
	thb, _ := json.Marshal(thBody)
	treq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(thb))
	treq.Header.Set("X-User-ID", user)
	treq.Header.Set("X-User-Signature", sig)
	treq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	tres, err := http.DefaultClient.Do(treq)
	if err != nil {
		t.Fatalf("create thread failed: %v", err)
	}
	defer tres.Body.Close()
	var tout map[string]interface{}
	if err := json.NewDecoder(tres.Body).Decode(&tout); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	tid := tout["id"].(string)

	// Create POST request to create a message under the thread
	req, err := http.NewRequest("POST", sp.Addr+"/v1/threads/"+tid+"/messages", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", sig)
	req.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	t.Logf("TestCreateMessage: request URL=%s, headers=%v", req.URL.String(), req.Header)

	// Send the request and check response
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	defer res.Body.Close()
	t.Logf("TestCreateMessage: response status=%v", res.Status)
	if res.StatusCode != 200 && res.StatusCode != 202 {
		t.Logf("TestCreateMessage: error response status=%v", res.Status)
		logResponseBody(t, res.Body, "TestCreateMessage error")
		t.Fatalf("expected 200 got %v", res.Status)
	}

	// Decode and validate response body
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	t.Logf("TestCreateMessage: response body=%v", out)
	if id, _ := out["id"].(string); id == "" {
		t.Fatalf("missing id in response")
	}
}

func TestListMessages(t *testing.T) {
	// Set up test server and user credentials
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "msg_list"
	sig := utils.SignHMAC(utils.SigningSecret, user)

	// Create a thread and message, and wait until the message is visible.
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "listme"}}
	thID, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "list-thread", 5*time.Second)
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, thID, payload, 5*time.Second)

	// List messages in the thread
	lreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+thID+"/messages", nil)
	lreq.Header.Set("X-User-ID", user)
	lreq.Header.Set("X-User-Signature", sig)
	lreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer lres.Body.Close()
	t.Logf("TestListMessages: response status=%v", lres.Status)
	if lres.StatusCode != 200 {
		logResponseBody(t, lres.Body, "TestListMessages error")
		t.Fatalf("expected 200 got %v", lres.Status)
	}

	// Decode and validate list response
	var listOut map[string]interface{}
	if err := json.NewDecoder(lres.Body).Decode(&listOut); err != nil {
		t.Fatalf("failed to decode list messages response: %v", err)
	}
	t.Logf("TestListMessages: response body=%v", listOut)
	if msgs, ok := listOut["messages"].([]interface{}); !ok || len(msgs) == 0 {
		t.Fatalf("expected messages in list result")
	}
}
