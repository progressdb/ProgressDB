package api

import (
	"bytes"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"

	"progressdb/pkg/store/messages"
	thread_store "progressdb/pkg/store/threads"
)

// auth handlers
func Sign(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.sign")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	logger.Info("signHandler called", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	if role != "backend" {
		logger.Warn("forbidden: non-backend role attempted to sign", "role", role, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	key := getAPIKey(ctx)
	if key == "" {
		logger.Warn("missing api key in signHandler", "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "missing api key")
		return
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&payload); err != nil {
		logger.Warn("invalid JSON payload in signHandler", "error", err, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Validate user ID format and content
	if err := ValidateUserID(payload.UserID); err != nil {
		logger.Warn("invalid user ID in signHandler", "error", err, "user_id", payload.UserID, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid user ID: %s", err.Error()))
		return
	}

	logger.Info("signing userId", "remote", ctx.RemoteAddr().String(), "role", role)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload.UserID))
	sig := hex.EncodeToString(mac.Sum(nil))

	tr.Mark("encode_response")
	if err := router.WriteJSON(ctx, map[string]string{"userId": payload.UserID, "signature": sig}); err != nil {
		logger.Error("failed to encode signHandler response", "error", err, "remote", ctx.RemoteAddr().String())
	}
}

// data handlers

func AdminHealth(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_health")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/json")
	_, _ = ctx.WriteString(`{"status":"ok","service":"progressdb"}`)
}

func AdminStats(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_stats")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/json")

	tr.Mark("list_threads")
	threadList, _ := thread_store.ListThreads()
	var msgCount int64
	tr.Mark("count_messages")
	for _, raw := range threadList {
		var th models.Thread
		if err := json.Unmarshal([]byte(raw), &th); err != nil {
			continue
		}
		indexes, err := index.GetThreadMessageIndexes(th.ID)
		if err != nil {
			continue
		}
		msgCount += int64(indexes.End - indexes.Start + 1)
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, struct {
		Threads  int   `json:"threads"`
		Messages int64 `json:"messages"`
	}{Threads: len(threadList), Messages: msgCount})
}

func AdminListThreads(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_list_threads")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/json")

	tr.Mark("list_threads")
	vals, err := thread_store.ListThreads()
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, struct {
		Threads []json.RawMessage `json:"threads"`
	}{Threads: router.ToRawMessages(vals)})
}

func AdminListKeys(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_list_keys")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/json")
	prefix := string(ctx.QueryArgs().Peek("prefix"))
	limitStr := string(ctx.QueryArgs().Peek("limit"))
	cursor := string(ctx.QueryArgs().Peek("cursor"))

	// Parse limit
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	tr.Mark("list_keys")
	result, err := listKeysByPrefixPaginated(prefix, limit, cursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, result)
}

func AdminGetKey(ctx *fasthttp.RequestCtx) {
	tr := telemetry.TrackWithStrategy("api.admin_get_key", telemetry.RotationStrategyPurge)
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
	keyEnc := pathParam(ctx, "key")
	if keyEnc == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing key")
		return
	}
	key, err := url.PathUnescape(keyEnc)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid key encoding")
		return
	}
	tr.Mark("get_key")
	val, err := storedb.GetKey(key)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/octet-stream")
	tr.Mark("write_response")
	_, _ = ctx.Write([]byte(val))
}

// hierarchical navigation handlers

func AdminListUsers(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_list_users")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	limitStr := string(ctx.QueryArgs().Peek("limit"))
	cursor := string(ctx.QueryArgs().Peek("cursor"))

	// Parse limit
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	tr.Mark("list_users")
	result, err := listUsersByPrefixPaginated("idx:U:", limit, cursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, result)
}

func AdminListUserThreads(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_list_user_threads")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	userID := pathParam(ctx, "userId")
	if userID == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing userId")
		return
	}

	limitStr := string(ctx.QueryArgs().Peek("limit"))
	cursor := string(ctx.QueryArgs().Peek("cursor"))

	// Parse limit
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	tr.Mark("list_user_threads")
	result, err := listThreadsForUser(userID, limit, cursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, result)
}

func AdminListThreadMessages(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_list_thread_messages")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	userID := pathParam(ctx, "userId")
	threadID := pathParam(ctx, "threadId")
	if userID == "" || threadID == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing userId or threadId")
		return
	}

	limitStr := string(ctx.QueryArgs().Peek("limit"))
	cursor := string(ctx.QueryArgs().Peek("cursor"))

	// Parse limit
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	tr.Mark("list_thread_messages")
	result, err := listMessagesForThread(threadID, limit, cursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, result)
}

func AdminGetThreadMessage(ctx *fasthttp.RequestCtx) {
	tr := telemetry.TrackWithStrategy("api.admin_get_thread_message", telemetry.RotationStrategyPurge)
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	userID := pathParam(ctx, "userId")
	threadID := pathParam(ctx, "threadId")
	messageID := pathParam(ctx, "messageId")
	if userID == "" || threadID == "" || messageID == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing userId, threadId, or messageId")
		return
	}

	tr.Mark("get_message")
	msg, err := messages.GetLatestMessage(messageID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

	tr.Mark("encode_response")
	ctx.Response.Header.Set("Content-Type", "application/json")
	_, _ = ctx.Write([]byte(msg))
}

// encryption handlers

func AdminEncryptionRotateThreadDEK(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_rotate_thread_dek")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
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
	tr.Mark("create_dek")
	newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(req.ThreadID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		auditLog("admin_rotate_thread_dek", map[string]interface{}{"thread_id": req.ThreadID, "status": "error", "error": err.Error()})
		return
	}
	tr.Mark("rotate_dek")
	if err := encryption.RotateThreadDEK(req.ThreadID, newKeyID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		auditLog("admin_rotate_thread_dek", map[string]interface{}{"thread_id": req.ThreadID, "new_key": newKeyID, "status": "error", "error": err.Error()})
		return
	}
	tr.Mark("update_thread")
	if s, err := thread_store.GetThread(req.ThreadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
			if payload, merr := json.Marshal(th); merr == nil {
				_ = thread_store.SaveThread(th.ID, string(payload))
			}
		}
	}
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, map[string]string{"status": "ok", "new_key": newKeyID})
	auditLog("admin_rotate_thread_dek", map[string]interface{}{"thread_id": req.ThreadID, "new_key": newKeyID, "status": "ok"})
}

func AdminEncryptionRewrapDEKs(ctx *fasthttp.RequestCtx) {
	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
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
	type result struct{ Key, Err, Kek string }
	resCh := make(chan result, len(keyIDs))
	for kid := range keyIDs {
		sem <- struct{}{}
		go func(id string) {
			defer func() { <-sem }()
			_, newKek, _, err := kms.RewrapDEKForThread(id, strings.TrimSpace(req.NewKEKHex))
			if err != nil {
				resCh <- result{Key: id, Err: err.Error()}
				return
			}
			resCh <- result{Key: id, Kek: newKek}
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
		if res.Err != "" {
			out[res.Key]["status"] = "error"
			out[res.Key]["error"] = res.Err
		} else {
			out[res.Key]["status"] = "ok"
			out[res.Key]["kek_id"] = res.Kek
		}
	}
	_ = router.WriteJSON(ctx, out)
	auditSummary("admin_rewrap_deks", len(threadsIDs), len(keyIDs), out)
}

func AdminEncryptionEncryptExisting(ctx *fasthttp.RequestCtx) {
	// check admin permissions
	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

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
	type result struct{ Thread, Key, Err string }
	resCh := make(chan result, len(threadsIDs))

	// iterate over threads and process in parallel
	for _, tid := range threadsIDs {
		sem <- struct{}{}
		go func(threadID string) {
			defer func() { <-sem }()
			// get thread metadata
			stored, err := thread_store.GetThread(threadID)
			if err != nil {
				resCh <- result{Thread: threadID, Err: "thread not found"}
				return
			}
			var th models.Thread
			if err := json.Unmarshal([]byte(stored), &th); err != nil {
				resCh <- result{Thread: threadID, Err: "invalid thread metadata"}
				return
			}

			// create a DEK for the thread if missing
			if th.KMS == nil || th.KMS.KeyID == "" {
				newKeyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadID)
				if err != nil {
					resCh <- result{Thread: threadID, Err: "create DEK failed: " + err.Error()}
					return
				}
				th.KMS = &models.KMSMeta{KeyID: newKeyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
				if payload, merr := json.Marshal(th); merr == nil {
					_ = thread_store.SaveThread(th.ID, string(payload))
				}
			}

			// get key prefix for thread messages
			prefix, merr := keys.MsgPrefix(threadID)
			if merr != nil {
				resCh <- result{Thread: threadID, Err: merr.Error()}
				return
			}

			// create iterator for all message keys in the thread
			iter, err := storedb.Iter()
			if err != nil {
				resCh <- result{Thread: threadID, Err: err.Error()}
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
						resCh <- result{Thread: threadID, Err: err.Error()}
						return
					}
					// backup original value
					backupKey := append([]byte("backup:encrypt:"), k...)
					if err := storedb.SaveKey(string(backupKey), v); err != nil {
						resCh <- result{Thread: threadID, Err: err.Error()}
						return
					}
					if err := storedb.Set(k, ct); err != nil {
						resCh <- result{Thread: threadID, Err: err.Error()}
						return
					}
				}
			}
			resCh <- result{Thread: threadID, Key: th.KMS.KeyID}
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
		if res.Err != "" {
			out[res.Thread]["status"] = "error"
			out[res.Thread]["error"] = res.Err
		} else {
			out[res.Thread]["status"] = "ok"
			out[res.Thread]["key_id"] = res.Key
		}
	}

	_ = router.WriteJSON(ctx, out)
	auditSummary("admin_encrypt_existing", len(threadsIDs), 0, out)
}

func AdminEncryptionGenerateKEK(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.admin_generate_kek")
	defer tr.Finish()

	if !isAdminUserRole(ctx) {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}
	tr.Mark("generate_key")
	buf := make([]byte, 32)
	if _, err := crand.Read(buf); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, "failed to generate key")
		return
	}
	kek := hex.EncodeToString(buf)
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, map[string]string{"kek_hex": kek})
	auditLog("admin_generate_kek", map[string]interface{}{"status": "ok"})
}

// helpers

func determineThreadIDs(ids []string, all bool) ([]string, error) {
	if all {
		vals, err := thread_store.ListThreads()
		if err != nil {
			return nil, err
		}
		threadsOut := make([]string, 0, len(vals))
		for _, raw := range vals {
			var th models.Thread
			if err := json.Unmarshal([]byte(raw), &th); err == nil {
				threadsOut = append(threadsOut, th.ID)
			}
		}
		return threadsOut, nil
	}
	return ids, nil
}

func auditSummary(event string, threads int, keys int, out map[string]map[string]string) {
	okCount := 0
	errCount := 0
	for _, m := range out {
		if s, ok := m["status"]; ok && s == "ok" {
			okCount++
		} else {
			errCount++
		}
	}
	fields := map[string]interface{}{"threads": threads, "ok": okCount, "errors": errCount}
	if keys > 0 {
		fields["keys"] = keys
	}
	auditLog(event, fields)
}

func isAdminUserRole(ctx *fasthttp.RequestCtx) bool {
	return string(ctx.Request.Header.Peek("X-Role-Name")) == "admin"
}

func auditLog(event string, fields map[string]interface{}) {
	if logger.Audit != nil {
		attrs := make([]interface{}, 0, len(fields)*2)
		for k, v := range fields {
			attrs = append(attrs, k, v)
		}
		logger.Audit.Info(event, attrs...)
		return
	}
	attrs := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}
	logger.Info(event, attrs...)
}

func getAPIKey(ctx *fasthttp.RequestCtx) string {
	auth := string(ctx.Request.Header.Peek("Authorization"))
	var key string
	if len(auth) > 7 && (auth[:7] == "Bearer " || auth[:7] == "bearer ") {
		key = auth[7:]
	}
	if key == "" {
		key = string(ctx.Request.Header.Peek("X-API-Key"))
	}
	return key
}

// AdminKeysResult represents paginated result for admin keys listing
type AdminKeysResult struct {
	Keys       []string `json:"keys"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}

// listKeysByPrefixPaginated lists database keys by prefix with cursor pagination (admin function)
func listKeysByPrefixPaginated(prefix string, limit int, cursor string) (*AdminKeysResult, error) {
	keys, nextCursor, hasMore, err := storedb.ListKeys(prefix, limit, cursor)
	if err != nil {
		return nil, err
	}

	return &AdminKeysResult{
		Keys:       keys,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Count:      len(keys),
	}, nil
}

// AdminUsersResult represents paginated result for admin users listing
type AdminUsersResult struct {
	Users      []string `json:"users"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}

// AdminThreadsResult represents paginated result for admin threads listing
type AdminThreadsResult struct {
	Threads    []json.RawMessage `json:"threads"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
	Count      int               `json:"count"`
}

// AdminMessagesResult represents paginated result for admin messages listing
type AdminMessagesResult struct {
	Messages   []json.RawMessage `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
	Count      int               `json:"count"`
}

// listUsersByPrefixPaginated lists users by prefix with cursor pagination (admin function)
func listUsersByPrefixPaginated(prefix string, limit int, cursor string) (*AdminUsersResult, error) {
	keys, nextCursor, hasMore, err := storedb.ListKeys(prefix, limit, cursor)
	if err != nil {
		return nil, err
	}

	// Extract user IDs from keys like "idx:U:user123:threads"
	users := make([]string, 0, len(keys))
	for _, key := range keys {
		// key format: idx:U:userID:threads
		parts := strings.Split(key, ":")
		if len(parts) >= 4 && parts[0] == "idx" && parts[1] == "U" && parts[3] == "threads" {
			users = append(users, parts[2])
		}
	}

	return &AdminUsersResult{
		Users:      users,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Count:      len(users),
	}, nil
}

// listThreadsForUser lists threads for a specific user with pagination
func listThreadsForUser(userID string, limit int, cursor string) (*AdminThreadsResult, error) {
	key, err := keys.UserThreadsIndexKey(userID)
	if err != nil {
		return nil, err
	}

	val, err := index.GetKey(key)
	if err != nil && !index.IsNotFound(err) {
		return nil, err
	}

	if val == "" {
		return &AdminThreadsResult{
			Threads:    []json.RawMessage{},
			NextCursor: "",
			HasMore:    false,
			Count:      0,
		}, nil
	}

	var indexes index.UserThreadIndexes
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal user threads: %w", err)
	}

	// Apply cursor and limit
	threadIDs := indexes.Threads
	start := 0
	if cursor != "" {
		for i, tid := range threadIDs {
			if tid == cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + limit
	if end > len(threadIDs) {
		end = len(threadIDs)
	}

	if start >= len(threadIDs) {
		return &AdminThreadsResult{
			Threads:    []json.RawMessage{},
			NextCursor: "",
			HasMore:    false,
			Count:      0,
		}, nil
	}

	pagedThreadIDs := threadIDs[start:end]
	threadList := make([]json.RawMessage, 0, len(pagedThreadIDs))

	for _, threadID := range pagedThreadIDs {
		threadData, err := thread_store.GetThread(threadID)
		if err != nil {
			continue // skip threads that can't be loaded
		}
		threadList = append(threadList, json.RawMessage(threadData))
	}

	nextCursor := ""
	if end < len(threadIDs) {
		nextCursor = threadIDs[end-1]
	}

	return &AdminThreadsResult{
		Threads:    threadList,
		NextCursor: nextCursor,
		HasMore:    end < len(threadIDs),
		Count:      len(threadList),
	}, nil
}

// listMessagesForThread lists messages for a specific thread with pagination
func listMessagesForThread(threadID string, limit int, cursor string) (*AdminMessagesResult, error) {
	messages, nextCursor, hasMore, err := messages.ListMessagesCursor(threadID, cursor, limit)
	if err != nil {
		return nil, err
	}

	return &AdminMessagesResult{
		Messages:   router.ToRawMessages(messages),
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Count:      len(messages),
	}, nil
}
