package api_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"progressdb/pkg/config"

	api "progressdb/pkg/api"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
)

func postJSONHandler(t *testing.T, h http.Handler, path string, v interface{}, headers map[string]string) []byte {
	t.Helper()
	b, _ := json.Marshal(v)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Body.Bytes()
}

func TestMessagesAndThreadsWorkflow(t *testing.T) {
	// Open a temporary pebble DB for the test
	tmp := t.TempDir()
	// Use a subdir to mirror typical usage
	dbdir := filepath.Join(tmp, "db")
	if err := store.Open(dbdir); err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	h := api.Handler()

	// configure runtime signing key used by the auth middleware
	rc := &config.RuntimeConfig{SigningKeys: map[string]struct{}{"sk_test": {}}, BackendKeys: map[string]struct{}{}}
	config.SetRuntime(rc)

	// helper signatures for users
	mac1 := hmac.New(sha256.New, []byte("sk_test"))
	mac1.Write([]byte("u1"))
	sig1 := hex.EncodeToString(mac1.Sum(nil))
	mac2 := hmac.New(sha256.New, []byte("sk_test"))
	mac2.Write([]byte("u2"))
	sig2 := hex.EncodeToString(mac2.Sum(nil))

	// Create a thread
	thrReq := map[string]interface{}{"name": "room-test"}
	thrBody := postJSONHandler(t, h, "/v1/threads", thrReq, map[string]string{"X-User-ID": "u1", "X-User-Signature": sig1})
	var thr models.Thread
	if err := json.Unmarshal(thrBody, &thr); err != nil {
		t.Fatalf("unmarshal thread: %v; body=%s", err, string(thrBody))
	}
	if thr.ID == "" {
		t.Fatalf("expected thread id")
	}

	// Post a message
	msgReq := map[string]interface{}{"thread": thr.ID, "author": "u1", "body": map[string]interface{}{"text": "hello"}}
	msgBody := postJSONHandler(t, h, "/v1/messages", msgReq, map[string]string{"X-User-ID": "u1", "X-User-Signature": sig1})
	var msg models.Message
	if err := json.Unmarshal(msgBody, &msg); err != nil {
		t.Fatalf("unmarshal message: %v; body=%s", err, string(msgBody))
	}
	if msg.ID == "" {
		t.Fatalf("expected message id")
	}

	// List messages for the thread
	reqList := httptest.NewRequest(http.MethodGet, "/v1/messages?thread="+thr.ID, nil)
	reqList.Header.Set("X-User-ID", "u1")
	reqList.Header.Set("X-User-Signature", sig1)
	recList := httptest.NewRecorder()
	h.ServeHTTP(recList, reqList)
	var list struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}
	if err := json.NewDecoder(recList.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Messages) == 0 {
		t.Fatalf("expected at least one message")
	}

	// Post a reply to the first message
	replyReq := map[string]interface{}{"thread": thr.ID, "author": "u2", "reply_to": msg.ID, "body": map[string]interface{}{"text": "reply here"}}
	replyBody := postJSONHandler(t, h, "/v1/messages", replyReq, map[string]string{"X-User-ID": "u2", "X-User-Signature": sig2})
	var reply models.Message
	if err := json.Unmarshal(replyBody, &reply); err != nil {
		t.Fatalf("unmarshal reply: %v; body=%s", err, string(replyBody))
	}
	if reply.ReplyTo != msg.ID {
		t.Fatalf("expected reply_to to be %s, got %s", msg.ID, reply.ReplyTo)
	}

	// Add a reaction using the reactions API
	reacReq := map[string]string{"id": "id-1", "reaction": "👍"}
	_ = postJSONHandler(t, h, "/v1/messages/"+msg.ID+"/reactions", reacReq, map[string]string{"X-User-ID": "u1", "X-User-Signature": sig1})

	// Verify reaction present on latest
	lrreq := httptest.NewRequest(http.MethodGet, "/v1/messages/"+msg.ID, nil)
	lrreq.Header.Set("X-User-ID", "u1")
	lrreq.Header.Set("X-User-Signature", sig1)
	lrc := httptest.NewRecorder()
	h.ServeHTTP(lrc, lrreq)
	var latestReact models.Message
	if err := json.NewDecoder(lrc.Body).Decode(&latestReact); err != nil {
		t.Fatalf("decode latest after reaction: %v", err)
	}
	if v, ok := latestReact.Reactions["id-1"]; !ok || v == "" {
		t.Fatalf("expected reaction for id-1 present, got %v", latestReact.Reactions)
	}

	// Edit the message (PUT) -> creates a new version
	msg.Body = map[string]interface{}{"text": "edited"}
	putBody, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPut, "/v1/messages/"+msg.ID, bytes.NewReader(putBody))
	req.Header.Set("X-User-ID", "u1")
	req.Header.Set("X-User-Signature", sig1)
	req.Header.Set("Content-Type", "application/json")
	prc := httptest.NewRecorder()
	h.ServeHTTP(prc, req)

	// Ensure versions list has at least 2 entries
	vreq := httptest.NewRequest(http.MethodGet, "/v1/messages/"+msg.ID+"/versions", nil)
	vreq.Header.Set("X-User-ID", "u1")
	vreq.Header.Set("X-User-Signature", sig1)
	vrec := httptest.NewRecorder()
	h.ServeHTTP(vrec, vreq)
	var vlist struct {
		ID       string   `json:"id"`
		Versions []string `json:"versions"`
	}
	if err := json.NewDecoder(vrec.Body).Decode(&vlist); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	if len(vlist.Versions) < 2 {
		t.Fatalf("expected >=2 versions, got %d", len(vlist.Versions))
	}

	// Delete (soft-delete)
	dreq := httptest.NewRequest(http.MethodDelete, "/v1/messages/"+msg.ID, nil)
	dreq.Header.Set("X-User-ID", "u1")
	dreq.Header.Set("X-User-Signature", sig1)
	drec := httptest.NewRecorder()
	h.ServeHTTP(drec, dreq)
	if drec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", drec.Code)
	}

	// Latest should reflect deleted=true
	lreq := httptest.NewRequest(http.MethodGet, "/v1/messages/"+msg.ID, nil)
	lreq.Header.Set("X-User-ID", "u1")
	lreq.Header.Set("X-User-Signature", sig1)
	lrec := httptest.NewRecorder()
	h.ServeHTTP(lrec, lreq)
	var latest models.Message
	if err := json.NewDecoder(lrec.Body).Decode(&latest); err != nil {
		t.Fatalf("decode latest: %v", err)
	}
	if !latest.Deleted {
		t.Fatalf("expected deleted=true on latest version")
	}

	// Threads list should include our thread
	treq := httptest.NewRequest(http.MethodGet, "/v1/threads", nil)
	treq.Header.Set("X-User-ID", "u1")
	treq.Header.Set("X-User-Signature", sig1)
	trec := httptest.NewRecorder()
	h.ServeHTTP(trec, treq)
	var tlist struct {
		Threads []json.RawMessage `json:"threads"`
	}
	if err := json.NewDecoder(trec.Body).Decode(&tlist); err != nil {
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
