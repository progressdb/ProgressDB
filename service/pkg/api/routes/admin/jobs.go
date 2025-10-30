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
	storedb "progressdb/pkg/store/db/storedb"
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

func encryptThreadWorker(threadKey string, sem chan struct{}, resCh chan DashboardEncryptJobResult) {
	defer func() { <-sem }()
	stored, err := thread_store.GetThread(threadKey)
	if err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: "thread not found", Success: false}
		return
	}
	var th models.Thread
	if err := json.Unmarshal([]byte(stored), &th); err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: "invalid thread metadata", Success: false}
		return
	}

	if th.KMS == nil || th.KMS.KeyID == "" {
		newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadKey)
		if err != nil {
			resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: "create DEK failed: " + err.Error(), Success: false}
			return
		}
		th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
		if payload, merr := json.Marshal(th); merr == nil {
			_ = saveThread(th.Key, string(payload))
		}
	}

	prefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: "failed to generate prefix: " + err.Error(), Success: false}
		return
	}

	iter, err := storedb.Iter()
	if err != nil {
		resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: err.Error(), Success: false}
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
				resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: err.Error(), Success: false}
				return
			}
			backupKey := append([]byte(keys.BackupEncryptPrefix), k...)
			if err := storedb.SaveKey(string(backupKey), v); err != nil {
				resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: err.Error(), Success: false}
				return
			}
			if err := storedb.Set(k, ct); err != nil {
				resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), Error: err.Error(), Success: false}
				return
			}
		}
	}
	resCh <- DashboardEncryptJobResult{Key: keys.GenThreadKey(threadKey), DEKKey: th.KMS.KeyID, Success: true}
}

func determineThreadKeys(ids []string, all bool) ([]string, error) {
	if all {
		prefix := keys.GenThreadMetadataPrefix()
		keyList, _, err := storedb.ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 10000, Cursor: ""})
		if err != nil {
			return nil, err
		}
		var threadKeys []string
		for _, k := range keyList {
			if parsed, err := keys.ParseKey(k); err == nil && parsed.Type == keys.KeyTypeThread {
				threadKeys = append(threadKeys, parsed.ThreadKey)
			}
		}
		return threadKeys, nil
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
	parsed, err := keys.ParseKey(req.Key)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread key")
		return
	}
	if parsed.Type != keys.KeyTypeThread {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "expected thread key")
		return
	}

	threadKey := parsed.ThreadKey
	newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if err := encryption.RotateThreadDEK(threadKey, newKeyID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if s, err := thread_store.GetThread(threadKey); err == nil {
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
			parsed, err := keys.ParseKey(k)
			if err != nil {
				router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread key")
				return
			}
			if parsed.Type != keys.KeyTypeThread {
				router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "expected thread key")
				return
			}
			threadIDs = append(threadIDs, parsed.ThreadKey)
		}
	}

	threadIDs, err := determineThreadKeys(threadIDs, req.All)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if len(threadIDs) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "no threads specified")
		return
	}

	keyIDs := make(map[string]struct{})
	for _, tkey := range threadIDs {
		if s, err := thread_store.GetThread(tkey); err == nil {
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
			parsed, err := keys.ParseKey(k)
			if err != nil {
				router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread key")
				return
			}
			if parsed.Type != keys.KeyTypeThread {
				router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "expected thread key")
				return
			}
			threadIDs = append(threadIDs, parsed.ThreadKey)
		}
	}

	threadIDs, err := determineThreadKeys(threadIDs, req.All)
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

	for _, tkey := range threadIDs {
		sem <- struct{}{}
		go encryptThreadWorker(tkey, sem, resCh)
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
