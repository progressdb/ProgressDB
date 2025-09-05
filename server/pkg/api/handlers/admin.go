package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"progressdb/pkg/models"
	"progressdb/pkg/store"

	"github.com/gorilla/mux"
)

// RegisterAdmin registers admin-only routes onto the admin subrouter.
func RegisterAdmin(r *mux.Router) {
	r.HandleFunc("/health", adminHealth).Methods(http.MethodGet)
	r.HandleFunc("/stats", adminStats).Methods(http.MethodGet)
	r.HandleFunc("/threads", adminListThreads).Methods(http.MethodGet)
	slog.Info("admin_routes_registered")
}

func adminHealth(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","service":"progressdb"}`))
}

func adminStats(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	threads, _ := store.ListThreads()
	var msgCount int64
	for _, tRaw := range threads {
		var th models.Thread
		if err := json.Unmarshal([]byte(tRaw), &th); err != nil {
			continue
		}
		msgs, err := store.ListMessages(th.ID)
		if err != nil {
			continue
		}
		msgCount += int64(len(msgs))
	}
	out := struct {
		Threads  int   `json:"threads"`
		Messages int64 `json:"messages"`
	}{Threads: len(threads), Messages: msgCount}
	_ = json.NewEncoder(w).Encode(out)
}

func adminListThreads(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	vals, err := store.ListThreads()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Threads []json.RawMessage `json:"threads"`
	}{Threads: toRawMessages(vals)})
}

// isAdmin checks if the request is from an admin or backend.
func isAdmin(r *http.Request) bool {
	role := r.Header.Get("X-Role-Name")
	return role == "admin" || role == "backend"
}
