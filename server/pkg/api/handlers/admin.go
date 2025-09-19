package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"progressdb/pkg/kms"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"
	"strings"

	"progressdb/pkg/logger"

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
	r.HandleFunc("/rewrap_batch", adminRewrapBatch).Methods(http.MethodPost)
	r.HandleFunc("/generate_kek", adminGenerateKEK).Methods(http.MethodPost)
	logger.Log.Info("admin_routes_registered")
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
    // persist metadata into thread record (canonical location)
    if s, err := store.GetThread(req.ThreadID); err == nil {
        var th models.Thread
        if err := json.Unmarshal([]byte(s), &th); err == nil {
            th.KMS = models.KMSMeta{KeyID: newKeyID, WrappedDEK: "", KEKID: kekID, KEKVersion: kekVer}
            if nb, merr := json.Marshal(th); merr == nil {
                _ = store.SaveThread(th.ID, string(nb))
            }
        }
    }
	// perform migration
	if err := store.RotateThreadDEK(req.ThreadID, newKeyID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "new_key": newKeyID})
}

// adminRewrapBatch triggers rewrap operations for DEKs related to threads.
// Request JSON:
// { "thread_ids": ["t1","t2"], "all": false, "new_kek_hex": "<hex>", "parallelism": 4 }
func adminRewrapBatch(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		ThreadIDs   []string `json:"thread_ids"`
		All         bool     `json:"all"`
		NewKEKHex   string   `json:"new_kek_hex"`
		Parallelism int      `json:"parallelism"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.NewKEKHex) == "" {
		http.Error(w, `{"error":"missing new_kek_hex"}`, http.StatusBadRequest)
		return
	}
	if req.Parallelism <= 0 {
		req.Parallelism = 4
	}

	// Determine thread IDs
	var threads []string
	if req.All {
		tvals, err := store.ListThreads()
		if err != nil {
			http.Error(w, `{"error":"failed list threads"}`, http.StatusInternalServerError)
			return
		}
		for _, t := range tvals {
			var th models.Thread
			if err := json.Unmarshal([]byte(t), &th); err != nil {
				continue
			}
			threads = append(threads, th.ID)
		}
	} else {
		threads = req.ThreadIDs
	}

	if len(threads) == 0 {
		http.Error(w, `{"error":"no threads specified"}`, http.StatusBadRequest)
		return
	}

	// Build unique list of key IDs from thread metadata
	keyIDs := make(map[string]struct{})
	for _, tid := range threads {
		// lookup thread meta
        // read canonical thread metadata to get key id
        if s, err := store.GetThread(tid); err == nil {
            var th models.Thread
            if err := json.Unmarshal([]byte(s), &th); err == nil {
                if th.KMS.KeyID != "" {
                    keyIDs[th.KMS.KeyID] = struct{}{}
                }
            }
        }
	}

	if len(keyIDs) == 0 {
		http.Error(w, `{"error":"no key mappings found for provided threads"}`, http.StatusBadRequest)
		return
	}

	// Create remote client bound to the configured endpoint
	endpoint := os.Getenv("PROGRESSDB_KMS_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:6820"
	}
	rc := kms.NewRemoteClient(endpoint)

	// Concurrency
	sem := make(chan struct{}, req.Parallelism)
	type res struct {
		Key string
		Err string
		Kek string
	}
	resCh := make(chan res, len(keyIDs))
	for kid := range keyIDs {
		sem <- struct{}{}
		go func(k string) {
			defer func() { <-sem }()
			_, newKek, _, err := rc.RewrapDEKForThread(k, strings.TrimSpace(req.NewKEKHex))
			if err != nil {
				resCh <- res{Key: k, Err: err.Error()}
				return
			}
			resCh <- res{Key: k, Kek: newKek}
		}(kid)
	}
	// wait for all
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	close(resCh)

	// gather results
	out := map[string]map[string]string{}
	for rres := range resCh {
		if _, ok := out[rres.Key]; !ok {
			out[rres.Key] = map[string]string{}
		}
		if rres.Err != "" {
			out[rres.Key]["status"] = "error"
			out[rres.Key]["error"] = rres.Err
		} else {
			out[rres.Key]["status"] = "ok"
			out[rres.Key]["kek_id"] = rres.Kek
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// adminGenerateKEK generates a new random 32-byte KEK and returns it as a
// 64-hex string in JSON: { "kek_hex": "..." }
func adminGenerateKEK(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	// generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		http.Error(w, `{"error":"failed to generate key"}`, http.StatusInternalServerError)
		return
	}
	kek := hex.EncodeToString(b)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"kek_hex": kek})
}

// isAdmin checks if the request is from an admin or backend.
func isAdmin(r *http.Request) bool {
	role := r.Header.Get("X-Role-Name")
	return role == "admin" || role == "backend"
}
