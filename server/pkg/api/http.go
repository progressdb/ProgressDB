package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"log/slog"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
	"progressdb/pkg/validation"
)

var idSeq uint64

// Handler returns an http.Handler with JSON endpoints:
// - POST /v1/messages: JSON body of models.Message (id/thread/author/ts/body)
// - GET  /v1/messages?thread=<id>&limit=<n>: list messages in thread
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPost:
			var m models.Message
			if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			if m.Thread == "" {
				m.Thread = genThreadID()
			}
			if m.ID == "" {
				m.ID = genID()
			}
			if m.TS == 0 {
				m.TS = time.Now().UTC().UnixNano()
			}
			if m.Author == "" {
				m.Author = "none"
			}
			if err := validation.ValidateMessage(m); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
				return
			}
			b, err := json.Marshal(m)
			if err != nil {
				http.Error(w, `{"error":"marshal failed"}`, http.StatusInternalServerError)
				return
			}
			if err := store.SaveMessage(m.Thread, m.ID, string(b)); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			slog.Info("message_created", "thread", m.Thread, "id", m.ID)
			_ = json.NewEncoder(w).Encode(m)
		case http.MethodGet:
			threadID := r.URL.Query().Get("thread")
			if threadID == "" {
				threadID = "default"
			}
			msgs, err := store.ListMessages(threadID)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			// Optional limit
			if limStr := r.URL.Query().Get("limit"); limStr != "" {
				if lim, err := strconv.Atoi(limStr); err == nil && lim >= 0 && lim < len(msgs) {
					msgs = msgs[len(msgs)-lim:]
				}
			}
			// Try to decode messages into structured model; fall back to raw.
			out := make([]models.Message, 0, len(msgs))
			for _, s := range msgs {
				var m models.Message
				if err := json.Unmarshal([]byte(s), &m); err == nil {
					out = append(out, m)
				} else {
					out = append(out, models.Message{ID: "", Thread: threadID, TS: 0, Body: s})
				}
			}
			slog.Info("messages_list", "thread", threadID, "count", len(out))
			_ = json.NewEncoder(w).Encode(struct {
				Thread   string           `json:"thread"`
				Messages []models.Message `json:"messages"`
			}{Thread: threadID, Messages: out})
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Message-by-id and versioning endpoints
	mux.HandleFunc("/v1/messages/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// path is /v1/messages/{id} or /v1/messages/{id}/versions
		p := r.URL.Path[len("/v1/messages/"):]
		if p == "" {
			http.Error(w, `{"error":"message id missing"}`, http.StatusBadRequest)
			return
		}
		// removed unused variable 's' in for loop
		// split
		// quick split
		parts := splitPath(p)
		id := parts[0]

		switch r.Method {
		case http.MethodGet:
			if len(parts) > 1 && parts[1] == "versions" {
				vs, err := store.ListMessageVersions(id)
				if err != nil {
					http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(struct {
					ID       string   `json:"id"`
					Versions []string `json:"versions"`
				}{ID: id, Versions: vs})
				return
			}
			s, err := store.GetLatestMessage(id)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(s))
		case http.MethodPut:
			var m models.Message
			if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			// enforce ID
			m.ID = id
			if m.Thread == "" {
				m.Thread = genThreadID()
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
		case http.MethodDelete:
			// soft-delete: append a tombstone version
			// fetch latest to get thread
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
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Simple root help
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"endpoints":["POST /v1/messages","GET /v1/messages?thread=<id>&limit=<n>"]}`))
	})

	// Threads: create/list
	mux.HandleFunc("/v1/threads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
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
			b, _ := json.Marshal(t)
			if err := store.SaveThread(t.ID, string(b)); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(t)
		case http.MethodGet:
			vals, err := store.ListThreads()
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(struct {
				Threads []json.RawMessage `json:"threads"`
			}{Threads: toRawMessages(vals)})
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/threads/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Path[len("/v1/threads/"):]
		if id == "" {
			http.Error(w, `{"error":"thread id missing"}`, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			s, err := store.GetThread(id)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(s))
		case http.MethodDelete:
			// Not implementing hard delete; just 204 if exists
			if _, err := store.GetThread(id); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	return mux
}

func genID() string {
	n := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&idSeq, 1)
	return fmt.Sprintf("msg-%d-%d", n, s)
}

func genThreadID() string {
	n := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&idSeq, 1)
	return fmt.Sprintf("thread-%d-%d", n, s)
}

// splitPath splits a path like "a/b/c" into components, trimming empty parts.
func splitPath(p string) []string {
	out := make([]string, 0)
	cur := ""
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func toRawMessages(vals []string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(vals))
	for _, s := range vals {
		out = append(out, json.RawMessage(s))
	}
	return out
}
