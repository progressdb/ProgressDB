package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"log/slog"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"
	"progressdb/pkg/validation"

	"github.com/gorilla/mux"
)

// RegisterMessages registers HTTP handlers for message-related endpoints.
func RegisterMessages(r *mux.Router) {
	// /v1/messages
	r.HandleFunc("/messages", createMessage).Methods(http.MethodPost)
	r.HandleFunc("/messages", listMessages).Methods(http.MethodGet)

	// /v1/messages/{id}
	r.HandleFunc("/messages/{id}", getMessage).Methods(http.MethodGet)
	r.HandleFunc("/messages/{id}", updateMessage).Methods(http.MethodPut)
	r.HandleFunc("/messages/{id}", deleteMessage).Methods(http.MethodDelete)
	r.HandleFunc("/messages/{id}/versions", listMessageVersions).Methods(http.MethodGet)

	// /v1/messages/{id}/reactions
	r.HandleFunc("/messages/{id}/reactions", getReactions).Methods(http.MethodGet)
	r.HandleFunc("/messages/{id}/reactions", addReaction).Methods(http.MethodPost)
	r.HandleFunc("/messages/{id}/reactions/{identity}", deleteReaction).Methods(http.MethodDelete)

	// /v1/threads/{threadID}/messages
	// thread-scoped message endpoints moved to handlers/threads.go

    // thread-scoped message-by-id endpoints moved to handlers/threads.go
}

// --- Handlers for /v1/messages ---
func createMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var m models.Message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if m.Thread == "" {
		m.Thread = utils.GenThreadID()
	}
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
	// ensure thread meta
	if sthr, err := store.GetThread(m.Thread); err != nil {
		th := models.Thread{ID: m.Thread, Title: "", Author: "", Slug: "", CreatedTS: m.TS, UpdatedTS: m.TS}
		_ = store.SaveThread(th.ID, func() string { b, _ := json.Marshal(th); return string(b) }())
	} else {
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err == nil {
			th.UpdatedTS = m.TS
			_ = store.SaveThread(th.ID, func() string { b, _ := json.Marshal(th); return string(b) }())
		}
	}
	slog.Info("message_created", "thread", m.Thread, "id", m.ID)
	_ = json.NewEncoder(w).Encode(m)
}

func listMessages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	threadID := r.URL.Query().Get("thread")
	if threadID == "" {
		threadID = "default"
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
	out := make([]models.Message, 0, len(msgs))
	for _, s := range msgs {
		var mm models.Message
		if err := json.Unmarshal([]byte(s), &mm); err == nil {
			out = append(out, mm)
		} else {
			out = append(out, models.Message{ID: "", Thread: threadID, TS: 0, Body: s})
		}
	}
	slog.Info("messages_list", "thread", threadID, "count", len(out))
	_ = json.NewEncoder(w).Encode(struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}{Thread: threadID, Messages: out})
}

func getMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	s, err := store.GetLatestMessage(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	_, _ = w.Write([]byte(s))
}

func updateMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	var m models.Message
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	m.ID = id
	if m.Thread == "" {
		m.Thread = utils.GenThreadID()
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
	// Ensure thread meta exists or update UpdatedTS
	if sthr, err := store.GetThread(m.Thread); err != nil {
		th := models.Thread{ID: m.Thread, Title: "", Author: "", Slug: "", CreatedTS: m.TS, UpdatedTS: m.TS}
		_ = store.SaveThread(th.ID, func() string { b, _ := json.Marshal(th); return string(b) }())
	} else {
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err == nil {
			th.UpdatedTS = m.TS
			_ = store.SaveThread(th.ID, func() string { b, _ := json.Marshal(th); return string(b) }())
		}
	}
	_ = json.NewEncoder(w).Encode(m)
}

func deleteMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	m.Deleted = true
	m.TS = time.Now().UTC().UnixNano()
	b, _ := json.Marshal(m)
	if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func listMessageVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	vs, err := store.ListMessageVersions(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		ID       string   `json:"id"`
		Versions []string `json:"versions"`
	}{ID: id, Versions: vs})
}

// --- Handlers for /v1/messages/{id}/reactions ---
func getReactions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	out := make([]struct {
		ID       string `json:"id"`
		Reaction string `json:"reaction"`
	}, 0)
	for k, v := range m.Reactions {
		out = append(out, struct {
			ID       string `json:"id"`
			Reaction string `json:"reaction"`
		}{ID: k, Reaction: v})
	}
	_ = json.NewEncoder(w).Encode(struct {
		ID        string      `json:"id"`
		Reactions interface{} `json:"reactions"`
	}{ID: id, Reactions: out})
}

func addReaction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := mux.Vars(r)["id"]
	var payload struct {
		ID       string `json:"id"`
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	identity := payload.ID
	if identity == "" {
		identity = r.Header.Get("X-Identity")
	}
	if identity == "" || payload.Reaction == "" {
		http.Error(w, `{"error":"missing id or reaction"}`, http.StatusBadRequest)
		return
	}
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
	if m.Reactions == nil {
		m.Reactions = map[string]string{}
	}
	m.Reactions[identity] = payload.Reaction
	m.TS = time.Now().UTC().UnixNano()
	b, _ := json.Marshal(m)
	if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(m)
}

func deleteReaction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]
	identity := vars["identity"]
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
	if m.Reactions != nil {
		delete(m.Reactions, identity)
	}
	m.TS = time.Now().UTC().UnixNano()
	b, _ := json.Marshal(m)
	if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// thread-scoped message handlers moved to handlers/threads.go
