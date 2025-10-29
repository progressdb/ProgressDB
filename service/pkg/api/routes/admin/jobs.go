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
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/encryption/kms"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

func rewrapDEKWorker(id string, newKEKHex string, sem chan struct{}, resCh chan DashboardRewrapJobResult) {
	defer func() { <-sem }()
	_, newKek, _, err := kms.RewrapDEKForThread(id, strings.TrimSpace(newKEKHex))
	if err != nil {
		resCh <- DashboardRewrapJobResult{Key: id, Error: err.Error(), Success: false}
		return
	}
	resCh <- DashboardRewrapJobResult{Key: id, NewKEK: newKek, Success: true}
}

func encryptThreadWorker(threadID string, sem chan struct{}, resCh chan DashboardEncryptJobResult) {
	defer func() { <-sem }()
	stored, err := thread_store.GetThread(threadID)
	if err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: "thread not found", Success: false}
		return
	}
	var th models.Thread
	if err := json.Unmarshal([]byte(stored), &th); err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: "invalid thread metadata", Success: false}
		return
	}

	if th.KMS == nil || th.KMS.KeyID == "" {
		newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadID)
		if err != nil {
			resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: "create DEK failed: " + err.Error(), Success: false}
			return
		}
		th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
		if payload, merr := json.Marshal(th); merr == nil {
			_ = saveThread(th.Key, string(payload))
		}
	}

	prefix := keys.GenAllThreadMessagesPrefix(threadID)

	iter, err := storedb.Iter()
	if err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: err.Error(), Success: false}
		return
	}
	defer iter.Close()
	pfx := []byte(prefix)

	for iter.SeekGE(pfx); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), pfx) {
			break
		}
		k := append([]byte(nil), iter.Key()...)
		v := append([]byte(nil), iter.Value()...)
		if encryption.LikelyJSON(v) {
			ct, _, err := kms.EncryptWithDEK(th.KMS.KeyID, v, nil)
			if err != nil {
				resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: err.Error(), Success: false}
				return
			}
			backupKey := append([]byte(keys.BackupEncryptPrefix), k...)
			if err := storedb.SaveKey(string(backupKey), v); err != nil {
				resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: err.Error(), Success: false}
				return
			}
			if err := storedb.Set(k, ct); err != nil {
				resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), Error: err.Error(), Success: false}
				return
			}
		}
	}
	resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadID), DEKKey: th.KMS.KeyID, Success: true}
}

func determineThreadIDs(ids []string, all bool) ([]string, error) {
	if all {
		prefix := keys.GenThreadMetadataPrefix()
		keyList, _, err := storedb.ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 10000, Cursor: ""})
		if err != nil {
			return nil, err
		}
		var threadIDs []string
		for _, k := range keyList {
			if parts, err := keys.ParseThreadKey(k); err == nil {
				threadIDs = append(threadIDs, parts.ThreadKey)
			}
		}
		return threadIDs, nil
	}
	return ids, nil
}

func EncryptionRotateThreadDEK(ctx *fasthttp.RequestCtx) {
	var req EncryptionRotateRequest
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing key")
		return
	}
	parts, err := keys.ParseThreadKey(req.Key)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread key")
		return
	}
	threadID := parts.ThreadKey
	newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if err := encryption.RotateThreadDEK(threadID, newKeyID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if s, err := thread_store.GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
			if payload, merr := json.Marshal(th); merr == nil {
				_ = saveThread(th.Key, string(payload))
			}
		}
	}
	_ = router.WriteJSON(ctx, map[string]string{"status": "ok", "new_key": newKeyID})
}

func EncryptionRewrapDEKs(ctx *fasthttp.RequestCtx) {
	var req EncryptionRewrapRequest
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.NewKEKHex) == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing new_kek_hex")
		return
	}
	if req.Parallelism <= 0 {
		req.Parallelism = 4
	}

	var threadIDs []string
	if !req.All {
		threadIDs = make([]string, 0, len(req.Keys))
		for _, k := range req.Keys {
			parts, err := keys.ParseThreadKey(k)
			if err != nil {
				router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread key")
				return
			}
			threadIDs = append(threadIDs, parts.ThreadKey)
		}
	}

	threadIDs, err := determineThreadIDs(threadIDs, req.All)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if len(threadIDs) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "no threads specified")
		return
	}

	keyIDs := make(map[string]struct{})
	for _, tid := range threadIDs {
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
		go rewrapDEKWorker(kid, req.NewKEKHex, sem, resCh)
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
}

func EncryptionEncryptExisting(ctx *fasthttp.RequestCtx) {
	var req EncryptionEncryptRequest
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		return
	}

	if req.Parallelism <= 0 {
		req.Parallelism = 4
	}

	var threadIDs []string
	if !req.All {
		threadIDs = make([]string, 0, len(req.Keys))
		for _, k := range req.Keys {
			parts, err := keys.ParseThreadKey(k)
			if err != nil {
				router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread key")
				return
			}
			threadIDs = append(threadIDs, parts.ThreadKey)
		}
	}

	threadIDs, err := determineThreadIDs(threadIDs, req.All)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if len(threadIDs) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "no threads specified")
		return
	}

	sem := make(chan struct{}, req.Parallelism)
	resCh := make(chan DashboardEncryptJobResult, len(threadIDs))

	for _, tid := range threadIDs {
		sem <- struct{}{}
		go encryptThreadWorker(tid, sem, resCh)
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
			out[res.Key]["dek_key_id"] = res.DEKKey
		}
	}

	_ = router.WriteJSON(ctx, out)
}

func EncryptionGenerateKEK(ctx *fasthttp.RequestCtx) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, "failed to generate key")
		return
	}
	kek := hex.EncodeToString(buf)
	_ = router.WriteJSON(ctx, map[string]string{"kek_hex": kek})
}
