package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"

	"progressdb/internal/retention"
	"progressdb/pkg/kms"
	"progressdb/pkg/models"
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
	// Encryption admin routes
	r.HandleFunc("/encryption/rotate-thread-dek", adminEncryptionRotateThreadDEK).Methods(http.MethodPost)
	r.HandleFunc("/encryption/rewrap-deks", adminEncryptionRewrapDEKs).Methods(http.MethodPost)
	r.HandleFunc("/encryption/generate-kek", adminEncryptionGenerateKEK).Methods(http.MethodPost)
	// Encrypt legacy (pre-encryption) messages: { all: bool, thread_ids: [], parallelism: 4 }
	r.HandleFunc("/encryption/encrypt-existing", adminEncryptionEncryptExisting).Methods(http.MethodPost)
    logger.Info("admin_routes_registered")

	// test-only retention trigger. The handler checks TESTING env var before
	// executing; registration is safe in production but the handler will refuse
	// to run unless tests explicitly enable it.
	r.HandleFunc("/test/retention-run", adminTestRetentionRun).Methods(http.MethodPost)
}

func adminHealth(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","service":"progressdb"}`))
}

func adminStats(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		utils.JSONError(w, http.StatusForbidden, "forbidden")
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
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	vals, err := store.ListThreads()
	if err != nil {
		utils.JSONError(w, http.StatusInternalServerError, err.Error())
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
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	prefix := r.URL.Query().Get("prefix")
	keys, err := store.ListKeys(prefix)
	if err != nil {
		utils.JSONError(w, http.StatusInternalServerError, err.Error())
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
		utils.JSONError(w, http.StatusForbidden, "forbidden")
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

func adminEncryptionRotateThreadDEK(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		if logger.Audit != nil {
			logger.Audit.Info("admin_rotate_thread_dek", "status", "error", "error", "invalid request")
		} else {
			logger.Info("admin_rotate_thread_dek", "status", "error", "error", "invalid request")
		}
		return
	}
	if req.ThreadID == "" {
		http.Error(w, `{"error":"missing thread_id"}`, http.StatusBadRequest)
		if logger.Audit != nil {
			logger.Audit.Info("admin_rotate_thread_dek", "status", "error", "error", "missing thread_id")
		} else {
			logger.Info("admin_rotate_thread_dek", "status", "error", "error", "missing thread_id")
		}
		return
	}
	// create new DEK for thread
	newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(req.ThreadID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		if logger.Audit != nil {
			logger.Audit.Info("admin_rotate_thread_dek", "thread_id", req.ThreadID, "status", "error", "error", err.Error())
		} else {
			logger.Info("admin_rotate_thread_dek", "thread_id", req.ThreadID, "status", "error", "error", err.Error())
		}
		return
	}
	// perform migration first (use the existing thread metadata/old key)
	if err := store.RotateThreadDEK(req.ThreadID, newKeyID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		if logger.Audit != nil {
			logger.Audit.Info("admin_rotate_thread_dek", "thread_id", req.ThreadID, "new_key", newKeyID, "status", "error", "error", err.Error())
		} else {
			logger.Info("admin_rotate_thread_dek", "thread_id", req.ThreadID, "new_key", newKeyID, "status", "error", "error", err.Error())
		}
		return
	}

	// persist wrapped DEK and KEK metadata into thread record (canonical location)
	if s, err := store.GetThread(req.ThreadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
			// If we have a wrapped value, persist it; otherwise the field will be empty.
			if nb, merr := json.Marshal(th); merr == nil {
				_ = store.SaveThread(th.ID, string(nb))
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "new_key": newKeyID})
	if logger.Audit != nil {
		logger.Audit.Info("admin_rotate_thread_dek", "thread_id", req.ThreadID, "new_key", newKeyID, "status", "ok")
	} else {
		logger.Info("admin_rotate_thread_dek", "thread_id", req.ThreadID, "new_key", newKeyID, "status", "ok")
	}
}

func adminTestRetentionRun(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	// Only allow this in test environments
	if v := os.Getenv("PROGRESSDB_TESTING"); v != "1" && strings.ToLower(v) != "true" {
		http.Error(w, `{"error":"test endpoint disabled"}`, http.StatusForbidden)
		return
	}
	if err := retention.RunImmediate(); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// adminEncryptionRewrapDEKs triggers rewrap operations for DEKs related to threads.
// Request JSON:
// { "thread_ids": ["t1","t2"], "all": false, "new_kek_hex": "<hex>", "parallelism": 4 }
func adminEncryptionRewrapDEKs(w http.ResponseWriter, r *http.Request) {
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
		utils.JSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.NewKEKHex) == "" {
		utils.JSONError(w, http.StatusBadRequest, "missing new_kek_hex")
		if logger.Audit != nil {
			logger.Audit.Info("admin_rewrap_deks", "status", "error", "error", "missing new_kek_hex")
		} else {
			logger.Info("admin_rewrap_deks", "status", "error", "error", "missing new_kek_hex")
		}
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
			utils.JSONError(w, http.StatusInternalServerError, "failed list threads")
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
		utils.JSONError(w, http.StatusBadRequest, "no threads specified")
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
				if th.KMS != nil && th.KMS.KeyID != "" {
					keyIDs[th.KMS.KeyID] = struct{}{}
				}
			}
		}
	}

	if len(keyIDs) == 0 {
		utils.JSONError(w, http.StatusBadRequest, "no key mappings found for provided threads")
		return
	}

	// Create remote client bound to the configured endpoint
	// Use registered provider via security bridge; avoid creating per-request remote clients.
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
			_, newKek, _, err := kms.RewrapDEKForThread(k, strings.TrimSpace(req.NewKEKHex))
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
	// emit audit summary
	if logger.Audit != nil {
		okCount := 0
		errCount := 0
		for _, m := range out {
			if s, ok := m["status"]; ok && s == "ok" {
				okCount++
			} else {
				errCount++
			}
		}
		logger.Audit.Info("admin_rewrap_deks", "threads", len(threads), "keys", len(keyIDs), "ok", okCount, "errors", errCount)
	} else {
		logger.Info("admin_rewrap_deks", "threads", len(threads), "keys", len(keyIDs))
	}
}

// adminEncryptionEncryptExisting encrypts legacy plaintext messages for threads that
// already have a DEK configured. Request JSON:
// { "thread_ids": ["t1","t2"], "all": false, "parallelism": 4 }
func adminEncryptionEncryptExisting(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req struct {
		ThreadIDs   []string `json:"thread_ids"`
		All         bool     `json:"all"`
		Parallelism int      `json:"parallelism"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.JSONError(w, http.StatusBadRequest, "invalid request")
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
		utils.JSONError(w, http.StatusBadRequest, "no threads specified")
		return
	}

	// Create remote client bound to the configured endpoint
	// Concurrency
	sem := make(chan struct{}, req.Parallelism)
	type res struct {
		Thread string
		Key    string
		Err    string
	}
	resCh := make(chan res, len(threads))
	for _, tid := range threads {
		sem <- struct{}{}
		go func(tid string) {
			defer func() { <-sem }()
			// lookup thread meta
			s, err := store.GetThread(tid)
			if err != nil {
				resCh <- res{Thread: tid, Err: "thread not found"}
				return
			}
			var th models.Thread
			if err := json.Unmarshal([]byte(s), &th); err != nil {
				resCh <- res{Thread: tid, Err: "invalid thread metadata"}
				return
			}
			if th.KMS == nil || th.KMS.KeyID == "" {
				// provision a DEK for this thread
				newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(tid)
				if err != nil {
					resCh <- res{Thread: tid, Err: "create DEK failed: " + err.Error()}
					return
				}
				th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
				if nb, merr := json.Marshal(th); merr == nil {
					_ = store.SaveThread(th.ID, string(nb))
				}
			} else {
				// already has DEK: skip provisioning and only report later
			}
			// iterate messages and encrypt plaintext ones
			mp, merr := store.MsgPrefix(tid)
			if merr != nil {
				resCh <- res{Thread: tid, Err: merr.Error()}
				return
			}
			prefix := []byte(mp)
			iter, err := store.DBIter()
			if err != nil {
				resCh <- res{Thread: tid, Err: err.Error()}
				return
			}
			defer iter.Close()
			for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
				if !bytes.HasPrefix(iter.Key(), prefix) {
					break
				}
				k := append([]byte(nil), iter.Key()...)
				v := append([]byte(nil), iter.Value()...)
				// If value looks like JSON, assume plaintext and encrypt it.
				if store.LikelyJSON(v) {
					ct, kv, err := kms.EncryptWithDEK(th.KMS.KeyID, v, nil)
					if err != nil {
						resCh <- res{Thread: tid, Err: err.Error()}
						return
					}
					// backup original
					backupKey := append([]byte("backup:encrypt:"), k...)
					if err := store.SaveKey(string(backupKey), v); err != nil {
						resCh <- res{Thread: tid, Err: err.Error()}
						return
					}
					// write new ciphertext
					if err := store.DBSet(k, ct); err != nil {
						resCh <- res{Thread: tid, Err: err.Error()}
						return
					}
					_ = kv // ignore key version here
				}
			}
			resCh <- res{Thread: tid, Key: th.KMS.KeyID}
		}(tid)
	}
	// wait for all
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	close(resCh)

	out := map[string]map[string]string{}
	for rres := range resCh {
		if _, ok := out[rres.Thread]; !ok {
			out[rres.Thread] = map[string]string{}
		}
		if rres.Err != "" {
			out[rres.Thread]["status"] = "error"
			out[rres.Thread]["error"] = rres.Err
		} else {
			out[rres.Thread]["status"] = "ok"
			out[rres.Thread]["key_id"] = rres.Key
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)

	// emit audit summary for encrypt-existing
	if logger.Audit != nil {
		okCount := 0
		errCount := 0
		for _, m := range out {
			if s, ok := m["status"]; ok && s == "ok" {
				okCount++
			} else {
				errCount++
			}
		}
		logger.Audit.Info("admin_encrypt_existing", "threads", len(threads), "ok", okCount, "errors", errCount)
	} else {
		logger.Info("admin_encrypt_existing", "threads", len(threads))
	}
}

// adminEncryptionGenerateKEK generates a new random 32-byte KEK and returns it as a
// 64-hex string in JSON: { "kek_hex": "..." }
func adminEncryptionGenerateKEK(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		utils.JSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	// generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		utils.JSONError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	kek := hex.EncodeToString(b)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"kek_hex": kek})
	// audit generation (do not log the kek value in audit to avoid leaking secret)
	if logger.Audit != nil {
		logger.Audit.Info("admin_generate_kek", "status", "ok")
	} else {
		logger.Info("admin_generate_kek", "status", "ok")
	}
}

// isAdmin checks if the request is from an admin.
// Backend keys should not be treated as admin; admin endpoints require the
// explicit admin role.
func isAdmin(r *http.Request) bool {
	role := r.Header.Get("X-Role-Name")
	return role == "admin"
}
