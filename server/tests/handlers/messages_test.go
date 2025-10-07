package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	utils "progressdb/tests/utils"
	"testing"
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
	srv := utils.SetupServer(t)
	defer srv.Close()
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

	// Create POST request to create a message
	req, err := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
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
	if res.StatusCode != 200 {
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
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msg_list"
	sig := utils.SignHMAC(utils.SigningSecret, user)

	// Create a message to ensure there is something to list
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "listme"}}
	b, _ := json.Marshal(payload)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	creq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create message request failed: %v", err)
	}
	defer cres.Body.Close()
	var cout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&cout); err != nil {
		t.Fatalf("failed to decode create message response: %v", err)
	}
	thread := cout["thread"].(string)

	// List messages in the thread
	lreq, _ := http.NewRequest("GET", srv.URL+"/v1/messages?thread="+thread, nil)
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
