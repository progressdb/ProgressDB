package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"progressdb/pkg/auth"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"
	"progressdb/pkg/validation"

	"github.com/gorilla/mux"
)

// RegisterThreads registers all thread-related HTTP routes to the provided router.
func RegisterThreads(r *mux.Router) {
	// Collection routes
	r.HandleFunc("/threads", createThread).Methods(http.MethodPost)
	r.HandleFunc("/threads", listThreads).Methods(http.MethodGet)
	r.HandleFunc("/threads/{id}", updateThread).Methods(http.MethodPut)

	// Single resource routes
	r.HandleFunc("/threads/{id}", getThread).Methods(http.MethodGet)
	r.HandleFunc("/threads/{id}", deleteThread).Methods(http.MethodDelete)

	// Thread-scoped messages
	r.HandleFunc("/threads/{threadID}/messages", createThreadMessage).Methods(http.MethodPost)
	r.HandleFunc("/threads/{threadID}/messages", listThreadMessages).Methods(http.MethodGet)

	// Thread-scoped message versions + reactions (migrated from message-level API)
	r.HandleFunc("/threads/{threadID}/messages/{id}/versions", listMessageVersions).Methods(http.MethodGet)

	// reactions moved under thread scope
	r.HandleFunc("/threads/{threadID}/messages/{id}/reactions", getReactions).Methods(http.MethodGet)
	r.HandleFunc("/threads/{threadID}/messages/{id}/reactions", addReaction).Methods(http.MethodPost)
	r.HandleFunc("/threads/{threadID}/messages/{id}/reactions/{identity}", deleteReaction).Methods(http.MethodDelete)

	// Thread-message-scoped endpoints
	r.HandleFunc("/threads/{threadID}/messages/{id}", getThreadMessage).Methods(http.MethodGet)
	r.HandleFunc("/threads/{threadID}/messages/{id}", updateThreadMessage).Methods(http.MethodPut)
	r.HandleFunc("/threads/{threadID}/messages/{id}", deleteThreadMessage).Methods(http.MethodDelete)
}

// createThread handles POST /threads to create a new thread.
// The request body must contain a JSON object representing the thread.
// The "author" field is required in the body.
func createThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var t models.Thread
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		utils.JSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// resolve canonical author (signature or backend-provided)
	if author, code, msg := auth.ResolveAuthorFromRequest(r, t.Author); code != 0 {
		utils.JSONError(w, code, msg)
		return
	} else {
		t.Author = author
	}
	created, err := createThreadInternal(t.Author, t.Title)
	if err != nil {
		utils.JSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(created)
}

// createThreadInternal performs the core thread creation logic without dealing
// with HTTP concerns. It provisions per-thread KMS metadata when encryption is
// enabled, saves the thread metadata, and returns the created Thread.
func createThreadInternal(author, title string) (models.Thread, error) {
	var t models.Thread
	if title == "" {
		title = defaultThreadTitle()
	}
	t.ID = utils.GenThreadID()
	t.Author = author
	t.Title = title
	t.Slug = utils.MakeSlug(t.Title, t.ID)
	t.CreatedTS = time.Now().UTC().UnixNano()
	t.UpdatedTS = t.CreatedTS
	// initialize per-thread sequence to zero; first message will bump this
	t.LastSeq = 0

	if security.EncryptionEnabled() {
		// Only provision per-thread KMS metadata when a KMS provider is
		// actually registered and enabled. This avoids creating KMS metadata
		// when encryption is disabled in config or when no provider is
		// available (tests expect no KMS metadata when encryption.use=false).
		if kms.IsProviderEnabled() {
			logger.Info("provisioning_thread_kms", "thread", t.ID)
			keyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(t.ID)
			if err != nil {
				return models.Thread{}, err
			}
			// set pointer so kms is omitted when nil
			t.KMS = &models.KMSMeta{KeyID: keyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
		} else {
			logger.Info("encryption_enabled_but_no_kms_provider", "thread", t.ID)
		}
	}

	b, _ := json.Marshal(t)
	if err := store.SaveThread(t.ID, string(b)); err != nil {
		return models.Thread{}, err
	}
	return t, nil
}

// listThreads handles GET /threads to retrieve a list of threads.
// The "author" query parameter is required and filters threads by the exact author name.
// Optional query parameters:
//   - "title": filters threads containing the given substring (case-insensitive) in the title.
//   - "slug": filters threads by the exact slug.
//
// The response is a JSON object with a "threads" field containing the filtered list.
func listThreads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// role := r.Header.Get("X-Role-Name")
	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		utils.JSONError(w, code, msg)
		return
	}
	titleQ := r.URL.Query().Get("title")
	slugQ := r.URL.Query().Get("slug")

	vals, err := store.ListThreads()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	var out []models.Thread
	for _, v := range vals {
		var th models.Thread
		if err := json.Unmarshal([]byte(v), &th); err != nil {
			continue
		}
		// hide deleted threads for non-admins
		role := r.Header.Get("X-Role-Name")
		if th.Deleted && role != "admin" {
			continue
		}
		if th.Author != authorQ {
			continue
		}
		if titleQ != "" && !strings.Contains(strings.ToLower(th.Title), strings.ToLower(titleQ)) {
			continue
		}
		if slugQ != "" && th.Slug != slugQ {
			continue
		}
		out = append(out, th)
	}

	_ = json.NewEncoder(w).Encode(struct {
		Threads []models.Thread `json:"threads"`
	}{Threads: out})
}

// getThread handles GET /threads/{id} to retrieve a single thread by its ID.
// Returns 404 if the thread does not exist.
// The "author" query parameter is required and must match the thread's author.
func getThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(w, `{"error":"thread id missing"}`, http.StatusBadRequest)
		return
	}

	// Prefer signature-verified author; for admin role allow listing without
	// an explicit author (admin may inspect all messages).
	role := r.Header.Get("X-Role-Name")
	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		if role == "admin" {
			// allow admin to proceed without an author filter
			authorQ = ""
		} else {
			http.Error(w, msg, code)
			return
		}
	}

	s, err := store.GetThread(id)
	if err != nil {
		utils.JSONError(w, http.StatusNotFound, err.Error())
		return
	}

	var th models.Thread
	if err := json.Unmarshal([]byte(s), &th); err != nil {
		utils.JSONError(w, http.StatusInternalServerError, "failed to parse thread")
		return
	}
	if th.Deleted && role != "admin" {
		utils.JSONError(w, http.StatusNotFound, "thread not found")
		return
	}
	if th.Author != authorQ {
		utils.JSONError(w, http.StatusForbidden, "author does not match")
		return
	}

	_, _ = w.Write([]byte(s))
}

// deleteThread handles DELETE /threads/{id} to delete a thread by its ID.
// Returns 404 if the thread does not exist.
// The "author" query parameter is required and must match the thread's author.
func deleteThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(w, `{"error":"thread id missing"}`, http.StatusBadRequest)
		return
	}

	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	// perform soft-delete via store helper (marks thread deleted + append tombstone)
	if err := store.SoftDeleteThread(id, authorQ); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updateThread handles PUT /threads/{id} to update thread metadata.
func updateThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(w, `{"error":"thread id missing"}`, http.StatusBadRequest)
		return
	}
	var t models.Thread
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		utils.JSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// resolve author (signature or backend-provided)
	author, code, msg := auth.ResolveAuthorFromRequest(r, t.Author)
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	// load existing thread
	s, err := store.GetThread(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	var th models.Thread
	if err := json.Unmarshal([]byte(s), &th); err != nil {
		http.Error(w, `{"error":"failed to parse thread"}`, http.StatusInternalServerError)
		return
	}
	role := r.Header.Get("X-Role-Name")
	if role != "admin" && th.Author != author {
		http.Error(w, `{"error":"author does not match"}`, http.StatusForbidden)
		return
	}
	// Apply updates: allow title changes
	if t.Title != "" {
		th.Title = t.Title
		th.Slug = utils.MakeSlug(th.Title, th.ID)
	}
	th.UpdatedTS = time.Now().UTC().UnixNano()
	nb, _ := json.Marshal(th)
	if err := store.SaveThread(th.ID, string(nb)); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(th)
}

// defaultThreadTitle generates a default title for new threads.
// It counts existing threads and returns "New Thread #<n>" where n is count+1.
// On error (e.g., store unavailable) it falls back to a generic "New Thread" label.
func defaultThreadTitle() string {
	vals, err := store.ListThreads()
	if err != nil {
		return "New Thread"
	}
	return fmt.Sprintf("New Thread #%d", len(vals)+1)
}

// createThreadMessage handles POST /threads/{threadID}/messages to create a new message in a thread.
// The request body must contain a JSON object representing the message.
// The "author" field is required in the body.
func createThreadMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	threadID := mux.Vars(r)["threadID"]
	var m models.Message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	// resolve canonical author (signature or backend-provided)
	if author, code, msg := auth.ResolveAuthorFromRequest(r, m.Author); code != 0 {
		http.Error(w, msg, code)
		return
	} else {
		m.Author = author
	}
	// Ensure role is present; default to "user" if omitted
	if m.Role == "" {
		m.Role = "user"
	}
	m.Thread = threadID
	// Always generate message IDs server-side
	m.ID = utils.GenID()
	if m.TS == 0 {
		m.TS = time.Now().UTC().UnixNano()
	}

	// Prevent posting into deleted threads (non-admin callers)
	if sthr, err := store.GetThread(m.Thread); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err == nil {
			if th.Deleted && r.Header.Get("X-Role-Name") != "admin" {
				http.Error(w, `{"error":"thread deleted"}`, http.StatusForbidden)
				return
			}
		}
	}
	if err := validation.ValidateMessage(m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	if err := store.SaveMessage(m.Thread, m.ID, m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(m)
}

// listThreadMessages handles GET /threads/{threadID}/messages to list messages in a thread.
// Optional query parameter "limit" restricts the number of most recent messages returned.
// The "author" query parameter is required.
func listThreadMessages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	role := r.Header.Get("X-Role-Name")
	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		if role == "admin" {
			// allow admin to list without providing an author filter
			authorQ = ""
		} else {
			http.Error(w, msg, code)
			return
		}
	}
	threadID := mux.Vars(r)["threadID"]
	// If the thread is soft-deleted, hide it from non-admin callers
	if sthr, err := store.GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err == nil {
			if th.Deleted && r.Header.Get("X-Role-Name") != "admin" {
				http.Error(w, `{"error":"thread not found"}`, http.StatusNotFound)
				return
			}
		}
	}
	msgs, err := store.ListMessages(threadID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	if limStr := r.URL.Query().Get("limit"); limStr != "" {
		if lim, err := strconv.Atoi(limStr); err == nil && lim >= 0 && lim < len(msgs) {
			msgs = msgs[len(msgs)-lim:]
		}
	}
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"
	latest := make(map[string]models.Message)
	authorFound := false
	for _, s := range msgs {
		var mm models.Message
		if err := json.Unmarshal([]byte(s), &mm); err != nil {
			continue
		}
		cur, ok := latest[mm.ID]
		if !ok || mm.TS >= cur.TS {
			latest[mm.ID] = mm
		}
	}
	out := make([]models.Message, 0, len(latest))
	for _, v := range latest {
		if v.Author == authorQ {
			authorFound = true
		}
		if v.Deleted && !includeDeleted {
			continue
		}
		out = append(out, v)
	}
	// sort by TS ascending
	sort.Slice(out, func(i, j int) bool { return out[i].TS < out[j].TS })
	// If there are no messages to return, respond with an empty list.
	// For admin role, do not enforce the authorFound check; admin may view
	// all messages in the thread.
	if len(out) == 0 {
		_ = json.NewEncoder(w).Encode(struct {
			Thread   string           `json:"thread"`
			Messages []models.Message `json:"messages"`
		}{Thread: threadID, Messages: out})
		return
	}
	if role != "admin" {
		if !authorFound {
			http.Error(w, `{"error":"author not found in any message in this thread"}`, http.StatusForbidden)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}{Thread: threadID, Messages: out})
}

// getThreadMessage handles GET /threads/{threadID}/messages/{id} to retrieve a single message by its ID.
// Returns 404 if the message does not exist.
// The "author" query parameter is required and must match the message's author.
func getThreadMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	id := mux.Vars(r)["id"]
	s, err := store.GetLatestMessage(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	var m models.Message
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		http.Error(w, `{"error":"failed to parse message"}`, http.StatusInternalServerError)
		return
	}
	if m.Author != authorQ {
		http.Error(w, `{"error":"author does not match"}`, http.StatusForbidden)
		return
	}
	_ = json.NewEncoder(w).Encode(m)
}

// updateThreadMessage handles PUT /threads/{threadID}/messages/{id} to update a message.
// The request body must contain a JSON object representing the message.
// The "author" field is required in the body and must match the author query parameter if provided.
func updateThreadMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	threadID := vars["threadID"]
	id := vars["id"]

	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	var m models.Message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	// require verified author and ensure body author (if present) matches
	// resolveAuthor ensured author is present; proceed
	if m.Author != "" && m.Author != authorQ {
		http.Error(w, `{"error":"author in body does not match verified author"}`, http.StatusForbidden)
		return
	}
	// enforce verified author
	m.Author = authorQ
	m.ID = id
	m.Thread = threadID
	if m.TS == 0 {
		m.TS = time.Now().UTC().UnixNano()
	}
	if err := validation.ValidateMessage(m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	if err := store.SaveMessage(m.Thread, m.ID, m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(m)
}

// deleteThreadMessage handles DELETE /threads/{threadID}/messages/{id} to mark a message as deleted.
// Returns 404 if the message does not exist.
// The "author" query parameter is required and must match the message's author.
func deleteThreadMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	authorQ, code, msg := auth.ResolveAuthorFromRequest(r, "")
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	id := mux.Vars(r)["id"]
	s, err := store.GetLatestMessage(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	var m models.Message
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		http.Error(w, `{"error":"invalid stored message"}`, http.StatusInternalServerError)
		return
	}
	// allow admin role to bypass author match
	role := r.Header.Get("X-Role-Name")
	if role != "admin" && m.Author != authorQ {
		http.Error(w, `{"error":"author does not match"}`, http.StatusForbidden)
		return
	}
	m.Deleted = true
	m.TS = time.Now().UTC().UnixNano()

	if err := store.SaveMessage(m.Thread, m.ID, m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
