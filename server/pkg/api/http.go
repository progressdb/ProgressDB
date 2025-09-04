package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "sync/atomic"
    "time"

    "progressdb/pkg/models"
    "progressdb/pkg/store"
    "log/slog"
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
            if err := store.SaveMessage(m.Thread, string(b)); err != nil {
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

    // Simple root help
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"endpoints":["POST /v1/messages","GET /v1/messages?thread=<id>&limit=<n>"]}`))
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
