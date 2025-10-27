package admin

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/kms"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/encryption"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/keys"
)

// AdminEncryptionRotateThreadDEK rotates the DEK for a thread
func AdminEncryptionRotateThreadDEK(ctx *fasthttp.RequestCtx) {
	var req struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		auditLog("admin_rotate_thread_dek", map[string]interface{}{"status": "error", "error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.ThreadID) == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing thread_id")
		auditLog("admin_rotate_thread_dek", map[string]interface{}{"status": "error", "error": "missing thread_id"})
		return
	}
	newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(req.ThreadID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		auditLog("admin_rotate_thread_dek", map[string]interface{}{"thread_id": req.ThreadID, "status": "error", "error": err.Error()})
		return
	}
	if err := encryption.RotateThreadDEK(req.ThreadID, newKeyID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		auditLog("admin_rotate_thread_dek", map[string]interface{}{"thread_id": req.ThreadID, "new_key": newKeyID, "status": "error", "error": err.Error()})
		return
	}
	if s, err := thread_store.GetThread(req.ThreadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
			if payload, merr := json.Marshal(th); merr == nil {
				_ = saveThread(th.Key, string(payload))
			}
		}
	}
	_ = router.WriteJSON(ctx, map[string]string{"status": "ok", "new_key": newKeyID})
	auditLog("admin_rotate_thread_dek", map[string]interface{}{"thread_id": req.ThreadID, "new_key": newKeyID, "status": "ok"})
}

// AdminEncryptionRewrapDEKs rewraps DEKs with a new KEK
func AdminEncryptionRewrapDEKs(ctx *fasthttp.RequestCtx) {
	var req struct {
		ThreadIDs   []string `json:"thread_ids"`
		All         bool     `json:"all"`
		NewKEKHex   string   `json:"new_kek_hex"`
		Parallelism int      `json:"parallelism"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.NewKEKHex) == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing new_kek_hex")
		auditLog("admin_rewrap_deks", map[string]interface{}{"status": "error", "error": "missing new_kek_hex"})
		return
	}
	if req.Parallelism <= 0 {
		req.Parallelism = 4
	}

	threadsIDs, err := determineThreadIDs(req.ThreadIDs, req.All)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if len(threadsIDs) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "no threads specified")
		return
	}

	keyIDs := make(map[string]struct{})
	for _, tid := range threadsIDs {
		if s, err := thread_store.GetThread(tid); err == nil {
			var th models.Thread
			if err := json.Unmarshal([]byte(s), &th); err == nil {
				if th.KMS != nil && th.KMS.KeyID != "" {
					keyIDs[th.KMS.KeyID] = struct{}{}
				}
			}
		}
	}

	if len(keyIDs) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "no key mappings found for provided threads")
		return
	}

	sem := make(chan struct{}, req.Parallelism)
	resCh := make(chan DashboardRewrapJobResult, len(keyIDs))
	for kid := range keyIDs {
		sem <- struct{}{}
		go func(id string) {
			defer func() { <-sem }()
			_, newKek, _, err := kms.RewrapDEKForThread(id, strings.TrimSpace(req.NewKEKHex))
			if err != nil {
				resCh <- DashboardRewrapJobResult{Key: id, Error: err.Error(), Success: false}
				return
			}
			resCh <- DashboardRewrapJobResult{Key: id, NewKEK: newKek, Success: true}
		}(kid)
	}
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	close(resCh)

	out := map[string]map[string]string{}
	for res := range resCh {
		if _, ok := out[res.Key]; !ok {
			out[res.Key] = map[string]string{}
		}
		if !res.Success {
			out[res.Key]["status"] = "error"
			out[res.Key]["error"] = res.Error
		} else {
			out[res.Key]["status"] = "ok"
			out[res.Key]["kek_id"] = res.NewKEK
		}
	}
	_ = router.WriteJSON(ctx, out)
	auditSummary("admin_rewrap_deks", len(threadsIDs), len(keyIDs), out)
}

// AdminEncryptionEncryptExisting encrypts existing unencrypted messages
func AdminEncryptionEncryptExisting(ctx *fasthttp.RequestCtx) {
	// decode request body
	var req struct {
		ThreadIDs   []string `json:"thread_ids"`
		All         bool     `json:"all"`
		Parallelism int      `json:"parallelism"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		return
	}

	// set default parallelism if not provided
	if req.Parallelism <= 0 {
		req.Parallelism = 4
	}

	// determine thread IDs to operate on
	threadsIDs, err := determineThreadIDs(req.ThreadIDs, req.All)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if len(threadsIDs) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "no threads specified")
		return
	}

	// setup concurrency controls and result channel
	sem := make(chan struct{}, req.Parallelism)
	resCh := make(chan DashboardEncryptJobResult, len(threadsIDs))

	// iterate over threads and process in parallel
	for _, tid := range threadsIDs {
		sem <- struct{}{}
		go func(threadID string) {
			defer func() { <-sem }()
			// get thread metadata
			stored, err := thread_store.GetThread(threadID)
			if err != nil {
				resCh <- DashboardEncryptJobResult{Thread: threadID, Error: "thread not found", Success: false}
				return
			}
			var th models.Thread
			if err := json.Unmarshal([]byte(stored), &th); err != nil {
				resCh <- DashboardEncryptJobResult{Thread: threadID, Error: "invalid thread metadata", Success: false}
				return
			}

			// create a DEK for the thread if missing
			if th.KMS == nil || th.KMS.KeyID == "" {
				newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadID)
				if err != nil {
					resCh <- DashboardEncryptJobResult{Thread: threadID, Error: "create DEK failed: " + err.Error(), Success: false}
					return
				}
				th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
				if payload, merr := json.Marshal(th); merr == nil {
					_ = saveThread(th.Key, string(payload))
				}
			}

			// get key prefix for thread messages
			prefix := keys.GenAllThreadMessagesPrefix(threadID)

			// create iterator for all message keys in the thread
			iter, err := storedb.Iter()
			if err != nil {
				resCh <- DashboardEncryptJobResult{Thread: threadID, Error: err.Error(), Success: false}
				return
			}
			defer iter.Close()
			pfx := []byte(prefix)

			// iterate and encrypt messages
			for iter.SeekGE(pfx); iter.Valid(); iter.Next() {
				if !bytes.HasPrefix(iter.Key(), pfx) {
					break
				}
				k := append([]byte(nil), iter.Key()...)
				v := append([]byte(nil), iter.Value()...)
				if encryption.LikelyJSON(v) {
					ct, _, err := kms.EncryptWithDEK(th.KMS.KeyID, v, nil)
					if err != nil {
						resCh <- DashboardEncryptJobResult{Thread: threadID, Error: err.Error(), Success: false}
						return
					}
					// backup original value
					backupKey := append([]byte(keys.BackupEncryptPrefix), k...)
					if err := storedb.SaveKey(string(backupKey), v); err != nil {
						resCh <- DashboardEncryptJobResult{Thread: threadID, Error: err.Error(), Success: false}
						return
					}
					if err := storedb.Set(k, ct); err != nil {
						resCh <- DashboardEncryptJobResult{Thread: threadID, Error: err.Error(), Success: false}
						return
					}
				}
			}
			resCh <- DashboardEncryptJobResult{Thread: threadID, Key: th.KMS.KeyID, Success: true}
		}(tid)
	}

	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	close(resCh)

	out := map[string]map[string]string{}
	for res := range resCh {
		if _, ok := out[res.Thread]; !ok {
			out[res.Thread] = map[string]string{}
		}
		if !res.Success {
			out[res.Thread]["status"] = "error"
			out[res.Thread]["error"] = res.Error
		} else {
			out[res.Thread]["status"] = "ok"
			out[res.Thread]["key_id"] = res.Key
		}
	}

	_ = router.WriteJSON(ctx, out)
	auditSummary("admin_encrypt_existing", len(threadsIDs), 0, out)
}

// AdminEncryptionGenerateKEK generates a new KEK
func AdminEncryptionGenerateKEK(ctx *fasthttp.RequestCtx) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, "failed to generate key")
		return
	}
	kek := hex.EncodeToString(buf)
	_ = router.WriteJSON(ctx, map[string]string{"kek_hex": kek})
	auditLog("admin_generate_kek", map[string]interface{}{"status": "ok"})
}
