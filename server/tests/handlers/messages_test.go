package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

func TestCreateMessage(t *testing.T) {
	// Set up test server and user credentials
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msg_create"
	sig := utils.SignHMAC("signsecret", user)

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
	t.Logf("TestCreateMessage: request URL=%s, headers=%v", req.URL.String(), req.Header)

	// Send the request and check response
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	defer res.Body.Close()
	t.Logf("TestCreateMessage: response status=%v", res.Status)
	if res.StatusCode != 200 {
		var errResp map[string]interface{}
		_ = json.NewDecoder(res.Body).Decode(&errResp)
		t.Logf("TestCreateMessage: error response body=%v", errResp)
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
	sig := utils.SignHMAC("signsecret", user)

	// Create a message to ensure there is something to list
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "listme"}}
	b, _ := json.Marshal(payload)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	cres, _ := http.DefaultClient.Do(creq)
	var cout map[string]interface{}
	_ = json.NewDecoder(cres.Body).Decode(&cout)
	thread := cout["thread"].(string)

	// List messages in the thread
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

	// Decode and validate list response
	var listOut map[string]interface{}
	_ = json.NewDecoder(lres.Body).Decode(&listOut)
	if msgs, ok := listOut["messages"].([]interface{}); !ok || len(msgs) == 0 {
		t.Fatalf("expected messages in list result")
	}
}

func TestGetMessage(t *testing.T) {
	// Set up test server and user credentials
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msg_get"
	sig := utils.SignHMAC("signsecret", user)

	// Create a message to retrieve
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "gimme"}}
	b, _ := json.Marshal(payload)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	cres, _ := http.DefaultClient.Do(creq)
	var cout map[string]interface{}
	_ = json.NewDecoder(cres.Body).Decode(&cout)
	id := cout["id"].(string)
	t.Logf("Created message ID: %s", id)

	// Retrieve the message by ID
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

	// Decode and validate retrieved message
	var got map[string]interface{}
	_ = json.NewDecoder(gres.Body).Decode(&got)
	if gotID, _ := got["id"].(string); gotID != id {
		t.Fatalf("expected id %s got %s", id, gotID)
	}
}

func TestUpdateMessage(t *testing.T) {
	// Set up test server and user credentials
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msg_update"
	sig := utils.SignHMAC("signsecret", user)

	// Create a message to update
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "old"}}
	b, _ := json.Marshal(payload)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	cres, _ := http.DefaultClient.Do(creq)
	var cout map[string]interface{}
	_ = json.NewDecoder(cres.Body).Decode(&cout)
	id := cout["id"].(string)

	// Wait briefly to ensure timestamp difference for versioning
	time.Sleep(10 * time.Millisecond)

	// Prepare and send update request
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

	// Decode and validate updated message
	var uout map[string]interface{}
	_ = json.NewDecoder(ures.Body).Decode(&uout)
	if body, ok := uout["body"].(map[string]interface{}); !ok || body["text"].(string) != "new" {
		t.Fatalf("expected updated body text")
	}
}

func TestDeleteMessage(t *testing.T) {
	// Set up test server and user credentials
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msg_delete"
	sig := utils.SignHMAC("signsecret", user)

	// Create a message to delete
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

	// Send DELETE request for the message
	dreq, _ := http.NewRequest("DELETE", srv.URL+"/v1/messages/"+id, nil)
	dreq.Header.Set("X-User-ID", user)
	dreq.Header.Set("X-User-Signature", sig)
	dres, _ := http.DefaultClient.Do(dreq)
	if dres.StatusCode != 204 {
		t.Fatalf("delete failed: %v", dres.Status)
	}

	// Ensure the deleted message is no longer in the thread's message list
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
	// Set up test server and user credentials
	srv := utils.SetupServer(t)
	defer srv.Close()
	user := "msg_versions"
	sig := utils.SignHMAC("signsecret", user)

	// Create a message to version
	payload := map[string]interface{}{"author": user, "body": map[string]string{"text": "v1"}}
	b, _ := json.Marshal(payload)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	cres, _ := http.DefaultClient.Do(creq)
	var cout map[string]interface{}
	_ = json.NewDecoder(cres.Body).Decode(&cout)
	id := cout["id"].(string)

	// Update the message to create a new version
	up := map[string]interface{}{"author": user, "body": map[string]string{"text": "v2"}}
	ub, _ := json.Marshal(up)
	ureq, _ := http.NewRequest("PUT", srv.URL+"/v1/messages/"+id, bytes.NewReader(ub))
	ureq.Header.Set("X-User-ID", user)
	ureq.Header.Set("X-User-Signature", sig)
	http.DefaultClient.Do(ureq)

	// List all versions of the message
	vreq, _ := http.NewRequest("GET", srv.URL+"/v1/messages/"+id+"/versions", nil)
	vreq.Header.Set("X-User-ID", user)
	vreq.Header.Set("X-User-Signature", sig)
	vres, _ := http.DefaultClient.Do(vreq)
	if vres.StatusCode != 200 {
		t.Fatalf("versions request failed: %v", vres.Status)
	}

	// Decode and validate versions response
	var vout map[string]interface{}
	_ = json.NewDecoder(vres.Body).Decode(&vout)
	if versions, ok := vout["versions"].([]interface{}); !ok || len(versions) < 2 {
		t.Fatalf("expected at least 2 versions")
	}
}
