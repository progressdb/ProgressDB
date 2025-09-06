package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"progressdb/pkg/models"
	"progressdb/pkg/store"
)

func postJSON(t *testing.T, client *http.Client, url string, v interface{}) []byte {
	t.Helper()
	b, _ := json.Marshal(v)
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return out
}

func TestMessagesAndThreadsWorkflow(t *testing.T) {
	// Open a temporary pebble DB for the test
	tmp := t.TempDir()
	// Use a subdir to mirror typical usage
	dbdir := filepath.Join(tmp, "db")
	if err := store.Open(dbdir); err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	srv := httptest.NewServer(Handler())
	defer srv.Close()
	client := srv.Client()

	// Create a thread
	thrReq := map[string]interface{}{"name": "room-test"}
	thrBody := postJSON(t, client, srv.URL+"/v1/threads", thrReq)
	var thr models.Thread
	if err := json.Unmarshal(thrBody, &thr); err != nil {
		t.Fatalf("unmarshal thread: %v; body=%s", err, string(thrBody))
	}
	if thr.ID == "" {
		t.Fatalf("expected thread id")
	}

	// Post a message
	msgReq := map[string]interface{}{"thread": thr.ID, "author": "u1", "body": map[string]interface{}{"text": "hello"}}
	msgBody := postJSON(t, client, srv.URL+"/v1/messages", msgReq)
	var msg models.Message
	if err := json.Unmarshal(msgBody, &msg); err != nil {
		t.Fatalf("unmarshal message: %v; body=%s", err, string(msgBody))
	}
	if msg.ID == "" {
		t.Fatalf("expected message id")
	}

	// List messages for the thread
	resp, err := client.Get(srv.URL + "/v1/messages?thread=" + thr.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	defer resp.Body.Close()
	var list struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Messages) == 0 {
		t.Fatalf("expected at least one message")
	}

	// Post a reply to the first message
	replyReq := map[string]interface{}{"thread": thr.ID, "author": "u2", "reply_to": msg.ID, "body": map[string]interface{}{"text": "reply here"}}
	replyBody := postJSON(t, client, srv.URL+"/v1/messages", replyReq)
	var reply models.Message
	if err := json.Unmarshal(replyBody, &reply); err != nil {
		t.Fatalf("unmarshal reply: %v; body=%s", err, string(replyBody))
	}
	if reply.ReplyTo != msg.ID {
		t.Fatalf("expected reply_to to be %s, got %s", msg.ID, reply.ReplyTo)
	}

	// Add a reaction by editing the message's reactions map
	gresp, err := client.Get(srv.URL + "/v1/messages/" + msg.ID)
	if err != nil {
		t.Fatalf("get for reaction: %v", err)
	}
	var toReact models.Message
	if err := json.NewDecoder(gresp.Body).Decode(&toReact); err != nil {
		gresp.Body.Close()
		t.Fatalf("decode for reaction: %v", err)
	}
	gresp.Body.Close()
	if toReact.Reactions == nil {
		toReact.Reactions = map[string]string{}
	}
	// For simple client-side reactions we store a string value (e.g., identity id or emoji).
	toReact.Reactions["id-1"] = "👍"
	putB, _ := json.Marshal(toReact)
	preq, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/messages/"+toReact.ID, bytes.NewReader(putB))
	preq.Header.Set("Content-Type", "application/json")
	presp2, err := client.Do(preq)
	if err != nil {
		t.Fatalf("put reaction: %v", err)
	}
	presp2.Body.Close()

	// Verify reaction present on latest
	lr, err := client.Get(srv.URL + "/v1/messages/" + msg.ID)
	if err != nil {
		t.Fatalf("get latest after reaction: %v", err)
	}
	var latestReact models.Message
	if err := json.NewDecoder(lr.Body).Decode(&latestReact); err != nil {
		lr.Body.Close()
		t.Fatalf("decode latest after reaction: %v", err)
	}
	lr.Body.Close()
	if v, ok := latestReact.Reactions["id-1"]; !ok || v == "" {
		t.Fatalf("expected reaction for id-1 present, got %v", latestReact.Reactions)
	}

	// Edit the message (PUT) -> creates a new version
	msg.Body = map[string]interface{}{"text": "edited"}
	putBody, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/messages/"+msg.ID, bytes.NewReader(putBody))
	req.Header.Set("Content-Type", "application/json")
	presp, err := client.Do(req)
	if err != nil {
		t.Fatalf("put message: %v", err)
	}
	presp.Body.Close()

	// Ensure versions list has at least 2 entries
	vresp, err := client.Get(srv.URL + "/v1/messages/" + msg.ID + "/versions")
	if err != nil {
		t.Fatalf("get versions: %v", err)
	}
	defer vresp.Body.Close()
	var vlist struct {
		ID       string   `json:"id"`
		Versions []string `json:"versions"`
	}
	if err := json.NewDecoder(vresp.Body).Decode(&vlist); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	if len(vlist.Versions) < 2 {
		t.Fatalf("expected >=2 versions, got %d", len(vlist.Versions))
	}

	// Delete (soft-delete)
	dreq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/messages/"+msg.ID, nil)
	dresp, err := client.Do(dreq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	dresp.Body.Close()
	if dresp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", dresp.StatusCode)
	}

	// Latest should reflect deleted=true
	lresp, err := client.Get(srv.URL + "/v1/messages/" + msg.ID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	defer lresp.Body.Close()
	var latest models.Message
	if err := json.NewDecoder(lresp.Body).Decode(&latest); err != nil {
		t.Fatalf("decode latest: %v", err)
	}
	if !latest.Deleted {
		t.Fatalf("expected deleted=true on latest version")
	}

	// Threads list should include our thread
	tresp, err := client.Get(srv.URL + "/v1/threads")
	if err != nil {
		t.Fatalf("get threads: %v", err)
	}
	defer tresp.Body.Close()
	var tlist struct {
		Threads []json.RawMessage `json:"threads"`
	}
	if err := json.NewDecoder(tresp.Body).Decode(&tlist); err != nil {
		t.Fatalf("decode threads: %v", err)
	}
	found := false
	for _, raw := range tlist.Threads {
		var tt models.Thread
		if err := json.Unmarshal(raw, &tt); err == nil && tt.ID == thr.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created thread not found in list")
	}
}
