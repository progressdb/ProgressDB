package api

import (
    "encoding/json"
    "net/http"

    "progressdb/pkg/store"
)

// Handler returns an http.Handler that appends a message (if provided)
// and lists the messages for a given thread.
//
// Usage:
//   GET /?thread=<id>&msg=<optional>
//   POST / with form fields: thread, msg
func Handler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Allow simple CORS for ease of local testing
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Content-Type", "application/json")

        // Parse inputs
        if err := r.ParseForm(); err != nil {
            http.Error(w, `{"error":"invalid form"}`, http.StatusBadRequest)
            return
        }

        threadID := r.FormValue("thread")
        if threadID == "" {
            threadID = "default"
        }

        if msg := r.FormValue("msg"); msg != "" {
            if err := store.SaveMessage(threadID, msg); err != nil {
                http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
                return
            }
        }

        msgs, err := store.ListMessages(threadID)
        if err != nil {
            http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
            return
        }

        _ = json.NewEncoder(w).Encode(struct {
            Thread   string   `json:"thread"`
            Messages []string `json:"messages"`
        }{Thread: threadID, Messages: msgs})
    })
}

