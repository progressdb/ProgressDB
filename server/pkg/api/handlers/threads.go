package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"progressdb/pkg/auth"
	"progressdb/pkg/models"
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

	// Single resource routes
	r.HandleFunc("/threads/{id}", getThread).Methods(http.MethodGet)
	r.HandleFunc("/threads/{id}", deleteThread).Methods(http.MethodDelete)

	// Thread-scoped messages
	r.HandleFunc("/threads/{threadID}/messages", createThreadMessage).Methods(http.MethodPost)
	r.HandleFunc("/threads/{threadID}/messages", listThreadMessages).Methods(http.MethodGet)

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
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	// derive author from verified signature middleware
	if authID := auth.AuthorIDFromContext(r.Context()); authID != "" {
		t.Author = authID
	} else {
		http.Error(w, `{"error":"author signature required"}`, http.StatusUnauthorized)
		return
	}
	if t.ID == "" {
		t.ID = utils.GenThreadID()
	}
	if t.CreatedTS == 0 {
		t.CreatedTS = time.Now().UTC().UnixNano()
	}
	if t.Slug == "" {
		t.Slug = utils.MakeSlug(t.Title, t.ID)
	}
	if t.UpdatedTS == 0 {
		t.UpdatedTS = t.CreatedTS
	}

	b, _ := json.Marshal(t)
	if err := store.SaveThread(t.ID, string(b)); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(t)
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

	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		// default to verified author when not provided
		authorQ = auth.AuthorIDFromContext(r.Context())
	}
	if authorQ == "" {
		http.Error(w, `{"error":"author query parameter is required"}`, http.StatusBadRequest)
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

	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		authorQ = auth.AuthorIDFromContext(r.Context())
	}
	if authorQ == "" {
		http.Error(w, `{"error":"author query parameter is required"}`, http.StatusBadRequest)
		return
	}

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
	if th.Author != authorQ {
		http.Error(w, `{"error":"author does not match"}`, http.StatusForbidden)
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

	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		http.Error(w, `{"error":"author query parameter is required"}`, http.StatusBadRequest)
		return
	}

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
	if th.Author != authorQ {
		http.Error(w, `{"error":"author does not match"}`, http.StatusForbidden)
		return
	}

	// If all checks pass, delete the thread (or mark as deleted, depending on implementation)
	if err := store.DeleteThread(id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	// enforce author from verified signature middleware
	if authID := auth.AuthorIDFromContext(r.Context()); authID != "" {
		m.Author = authID
	} else {
		http.Error(w, `{"error":"author field is required"}`, http.StatusUnauthorized)
		return
	}
	m.Thread = threadID
	if m.ID == "" {
		m.ID = utils.GenID()
	}
	if m.TS == 0 {
		m.TS = time.Now().UTC().UnixNano()
	}
	if err := validation.ValidateMessage(m); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	b, _ := json.Marshal(m)
	if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
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
	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		authorQ = auth.AuthorIDFromContext(r.Context())
	}
	if authorQ == "" {
		http.Error(w, `{"error":"author query parameter is required"}`, http.StatusBadRequest)
		return
	}
	threadID := mux.Vars(r)["threadID"]
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
	out := make([]models.Message, 0, len(msgs))
	authorFound := false
	for _, s := range msgs {
		var mm models.Message
		if err := json.Unmarshal([]byte(s), &mm); err == nil {
			if mm.Author == authorQ {
				authorFound = true
			}
			out = append(out, mm)
		} else {
			// If message can't be unmarshaled, skip it
			continue
		}
	}
	if !authorFound {
		http.Error(w, `{"error":"author not found in any message in this thread"}`, http.StatusForbidden)
		return
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
	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		authorQ = auth.AuthorIDFromContext(r.Context())
	}
	if authorQ == "" {
		http.Error(w, `{"error":"author query parameter is required"}`, http.StatusBadRequest)
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

	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		authorQ = auth.AuthorIDFromContext(r.Context())
	}
	var m models.Message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	// require verified author and ensure body author (if present) matches
	if authorQ == "" {
		http.Error(w, `{"error":"author required"}`, http.StatusBadRequest)
		return
	}
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
	b, _ := json.Marshal(m)
	if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
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
	authorQ := r.URL.Query().Get("author")
	if authorQ == "" {
		authorQ = auth.AuthorIDFromContext(r.Context())
	}
	if authorQ == "" {
		http.Error(w, `{"error":"author query parameter is required"}`, http.StatusBadRequest)
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
	b, _ := json.Marshal(m)
	if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
