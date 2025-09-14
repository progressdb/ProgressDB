package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"

	"github.com/gorilla/mux"
)

// RegisterAdmin registers admin-only routes onto the admin subrouter.
func RegisterAdmin(r *mux.Router) {
	r.HandleFunc("/health", adminHealth).Methods(http.MethodGet)
	r.HandleFunc("/stats", adminStats).Methods(http.MethodGet)
	r.HandleFunc("/threads", adminListThreads).Methods(http.MethodGet)
	r.HandleFunc("/keys", adminListKeys).Methods(http.MethodGet)
	r.HandleFunc("/keys/{key}", adminGetKey).Methods(http.MethodGet)
	r.HandleFunc("/rotate_thread_dek", adminRotateThreadDEK).Methods(http.MethodPost)
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
	}{Threads: utils.ToRawMessages(vals)})
}

// adminListKeys lists keys in the underlying store. Optional query param
// `prefix` can be provided to limit keys by prefix.
func adminListKeys(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	prefix := r.URL.Query().Get("prefix")
	keys, err := store.ListKeys(prefix)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Keys []string `json:"keys"`
	}{Keys: keys})
}

// adminGetKey returns the raw value for a given key. The key path variable
// is URL-unescaped before lookup.
func adminGetKey(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	vars := mux.Vars(r)
	keyEnc, ok := vars["key"]
	if !ok {
		http.Error(w, `{"error":"missing key"}`, http.StatusBadRequest)
		return
	}
	// URL path variables are not automatically unescaped by gorilla/mux,
	// so use PathUnescape to recover the original key string.
	key, err := url.PathUnescape(keyEnc)
	if err != nil {
		http.Error(w, `{"error":"invalid key encoding"}`, http.StatusBadRequest)
		return
	}
	v, err := store.GetKey(key)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write([]byte(v))
}

func adminRotateThreadDEK(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.ThreadID == "" {
		http.Error(w, `{"error":"missing thread_id"}`, http.StatusBadRequest)
		return
	}
	// create new DEK for thread
    newKeyID, _, kekID, kekVer, err := security.CreateDEKForThread(req.ThreadID)
    if err != nil {
        http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
        return
    }
    // persist metadata
    meta := map[string]string{"key_id": newKeyID, "kek_id": kekID, "kek_version": kekVer}
    if mb, merr := json.Marshal(meta); merr == nil {
        _ = store.SaveKey("kms:map:threadmeta:"+req.ThreadID, mb)
    }
	// perform migration
	if err := store.RotateThreadDEK(req.ThreadID, newKeyID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "new_key": newKeyID})
}

// isAdmin checks if the request is from an admin or backend.
func isAdmin(r *http.Request) bool {
	role := r.Header.Get("X-Role-Name")
	return role == "admin" || role == "backend"
}
