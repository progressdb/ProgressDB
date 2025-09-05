package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"progressdb/pkg/models"
	"progressdb/pkg/store"

	"github.com/gorilla/mux"
)

// RegisterThreads registers thread-related routes.
func RegisterThreads(r *mux.Router) {
	// Collection routes
	r.HandleFunc("/threads", createThread).Methods(http.MethodPost)
	r.HandleFunc("/threads", listThreads).Methods(http.MethodGet)

	// Single resource routes
	r.HandleFunc("/threads/{id}", getThread).Methods(http.MethodGet)
	r.HandleFunc("/threads/{id}", deleteThread).Methods(http.MethodDelete)
}

func createThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var t models.Thread
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if t.ID == "" {
		t.ID = genThreadID()
	}
	if t.CreatedTS == 0 {
		t.CreatedTS = time.Now().UTC().UnixNano()
	}
	if t.Slug == "" {
		t.Slug = makeSlug(t.Title, t.ID)
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

func listThreads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	authorQ := r.URL.Query().Get("author")
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
		if authorQ != "" && th.Author != authorQ {
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

func getThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(w, `{"error":"thread id missing"}`, http.StatusBadRequest)
		return
	}

	s, err := store.GetThread(id)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	_, _ = w.Write([]byte(s))
}

func deleteThread(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := mux.Vars(r)["id"]
	if id == "" {
		http.Error(w, `{"error":"thread id missing"}`, http.StatusBadRequest)
		return
	}

	if _, err := store.GetThread(id); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
