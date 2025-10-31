package admin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"

	"github.com/cockroachdb/pebble"
	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"progressdb/pkg/state/logger"
)

func Health(ctx *fasthttp.RequestCtx) {
	router.WriteJSONOk(ctx, map[string]interface{}{
		"status":  "ok",
		"service": "progressdb",
	})
}

func Stats(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// Get all thread metadata keys using prefix pagination
	var allKeys []string
	cursor := ""
	for {
		prefix := keys.GenThreadMetadataPrefix()
		keys, resp, err := storedb.ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 100, Cursor: cursor})
		if err != nil {
			router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
		allKeys = append(allKeys, keys...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}

	// Filter only thread keys and get their values
	var threadCount int
	var msgCount int64
	for _, key := range allKeys {
		parsed, err := keys.ParseKey(key)
		if err != nil {
			continue
		}
		if parsed.Type == keys.KeyTypeThread {
			threadCount++
			// Get thread value to count messages
			val, err := storedb.GetKey(key)
			if err != nil {
				continue
			}
			var th models.Thread
			if err := json.Unmarshal([]byte(val), &th); err == nil {
				indexes, err := indexdb.GetThreadMessageIndexes(th.Key)
				if err == nil {
					msgCount += int64(indexes.End)
				}
			}
		}
	}

	_ = router.WriteJSON(ctx, struct {
		Threads  int   `json:"threads"`
		Messages int64 `json:"messages"`
	}{Threads: threadCount, Messages: msgCount})
}

func ListThreads(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// Get all thread metadata keys using prefix pagination
	var allKeys []string
	cursor := ""
	for {
		prefix := keys.GenThreadMetadataPrefix()
		keys, resp, err := storedb.ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 100, Cursor: cursor})
		if err != nil {
			router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
		allKeys = append(allKeys, keys...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}

	// Filter only thread keys and get their values
	var threads []string
	for _, key := range allKeys {
		parsed, err := keys.ParseKey(key)
		if err != nil {
			continue
		}
		if parsed.Type == keys.KeyTypeThread {
			val, err := storedb.GetKey(key)
			if err != nil {
				continue
			}
			threads = append(threads, val)
		}
	}

	_ = router.WriteJSON(ctx, struct {
		Threads []string `json:"threads"`
	}{Threads: threads})
}

func ListKeys(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	prefix := string(ctx.QueryArgs().Peek("prefix"))
	store := string(ctx.QueryArgs().Peek("store"))
	if store == "" {
		store = "main"
	}
	paginationReq := pagination.ParsePaginationRequest(ctx)

	// Use direct database call with prefix pagination
	var keys []string
	var paginationResp *pagination.PaginationResponse
	var err error

	if store == "index" {
		keys, paginationResp, err = indexdb.ListKeysWithPrefixPaginated(prefix, paginationReq)
	} else {
		keys, paginationResp, err = storedb.ListKeysWithPrefixPaginated(prefix, paginationReq)
	}

	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	result := &DashboardKeysResult{
		Keys:       keys,
		Pagination: paginationResp,
	}
	_ = router.WriteJSON(ctx, result)
}

func GetKey(ctx *fasthttp.RequestCtx) {
	keyEnc, ok := extractParamOrFail(ctx, "key", "missing key")
	if !ok {
		return
	}
	key, err := url.PathUnescape(keyEnc)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid key encoding")
		return
	}
	storeParam, _ := extractQueryOrFail(ctx, "store", "")

	logger.Debug("GetKey: storeParam", storeParam)
	var val string
	switch storeParam {
	case "index":
		val, err = indexdb.GetKey(key)
	default:
		val, err = storedb.GetKey(key)
	}
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/octet-stream")
	_, _ = ctx.Write([]byte(val))
}

func ListUsers(ctx *fasthttp.RequestCtx) {
	// Use direct database iteration with prefix bounds
	lowerBound := []byte(keys.UserThreadsRelPrefix)
	upperBound := nextPrefix(lowerBound)

	iter, err := indexdb.Client.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	defer iter.Close()

	userSet := make(map[string]struct{})

	for valid := iter.First(); valid; valid = iter.Next() {
		keyStr := string(iter.Key())

		// Use unified parser
		parsed, err := keys.ParseKey(keyStr)
		if err != nil {
			continue // Skip invalid keys
		}

		// Only collect user IDs from user-thread relationships
		if parsed.Type == keys.KeyTypeUserOwnsThread && parsed.UserID != "" {
			userSet[parsed.UserID] = struct{}{}
		}
	}

	// Convert userSet keys to sorted slice for output
	allUsers := make([]string, 0, len(userSet))
	for user := range userSet {
		allUsers = append(allUsers, user)
	}
	sort.Strings(allUsers)

	result := &DashboardUsersResult{
		Users:      allUsers,
		Pagination: nil,
	}
	_ = router.WriteJSON(ctx, result)
}

func ListUserThreads(ctx *fasthttp.RequestCtx) {
	userID, ok := extractParamOrFail(ctx, "userId", "missing userId")
	if !ok {
		return
	}

	// Validate userID
	if err := keys.ValidateUserID(userID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// Use direct database iteration with prefix bounds
	prefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	lowerBound := []byte(prefix)
	upperBound := nextPrefix(lowerBound)

	iter, err := indexdb.Client.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	defer iter.Close()

	var threadKeys []string
	count := 0
	const maxThreads = 100

	for valid := iter.First(); valid && count < maxThreads; valid = iter.Next() {
		keyStr := string(iter.Key())

		// Use unified parser
		parsed, err := keys.ParseKey(keyStr)
		if err != nil {
			logger.Debug("skipping invalid key %q: %v", keyStr, err)
			continue
		}

		// Only process user-thread relationships for this user
		if parsed.Type != keys.KeyTypeUserOwnsThread || parsed.UserID != userID {
			continue
		}

		threadKeys = append(threadKeys, parsed.ThreadKey)
		count++
	}

	result := &DashboardThreadsResult{
		Threads:    threadKeys,
		Pagination: nil,
	}
	_ = router.WriteJSON(ctx, result)
}

func ListThreadMessages(ctx *fasthttp.RequestCtx) {
	threadKey, ok := extractParamOrFail(ctx, "threadKey", "missing threadKey")
	if !ok {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing or invalid threadKey")
		return
	}
	logger.Debug("ListThreadMessages: threadKey =", threadKey)

	// Parse and validate thread key using unified parser
	parsedThread, err := keys.ParseKey(threadKey)
	if err != nil {
		logger.Error("ListThreadMessages: invalid thread key", threadKey, err)
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	if parsedThread.Type != keys.KeyTypeThread {
		logger.Error("ListThreadMessages: not a thread key", threadKey)
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Errorf("expected thread key, got %s", parsedThread.Type).Error())
		return
	}

	// Generate prefix for messages of this thread using the thread timestamp
	prefix, err := keys.GenAllThreadMessagesPrefix(parsedThread.ThreadKey)
	if err != nil {
		logger.Error("ListThreadMessages: failed to generate prefix:", err)
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	logger.Debug("ListThreadMessages: generated prefix =", prefix)
	lowerBound := []byte(prefix)
	upperBound := nextPrefix(lowerBound)

	logger.Debug("ListThreadMessages: using range lowerBound=", string(lowerBound), "upperBound=", string(upperBound))

	// Use direct database iteration
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		logger.Error("ListThreadMessages: failed to create iterator:", err)
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	defer iter.Close()

	var msgKeys []string
	count := 0
	limit := 100 // default

	for valid := iter.First(); valid && count < limit; valid = iter.Next() {
		messagekey := string(iter.Key())

		// Use unified parser to validate and extract message info
		parsedMsg, err := keys.ParseKey(messagekey)
		if err != nil {
			logger.Debug("ListThreadMessages: skipping invalid message key", messagekey, err)
			continue
		}

		// Only include message keys (not provisional) for this thread
		if parsedMsg.Type != keys.KeyTypeMessage || parsedMsg.ThreadKey != parsedThread.ThreadKey {
			logger.Debug("ListThreadMessages: skipping non-matching message", messagekey)
			continue
		}

		logger.Debug("ListThreadMessages: found message", messagekey)
		msgKeys = append(msgKeys, messagekey)
		count++
	}
	logger.Info("ListThreadMessages: found", count, "messages for thread", threadKey)

	result := &DashboardMessagesResult{
		Messages:   msgKeys,
		Pagination: pagination.NewPaginationResponse(limit, count == limit, "", count, count),
	}
	_ = router.WriteJSON(ctx, result)
}

func GetThreadMessage(ctx *fasthttp.RequestCtx) {
	messageKey, ok := extractParamOrFail(ctx, "messageKey", "missing messageKey")
	if !ok {
		return
	}

	val, err := storedb.GetKey(messageKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

	ctx.Response.Header.Set("Content-Type", "application/json")
	_, _ = ctx.Write([]byte(val))
}

// Helper for upper bound: gets next lexicographical key after prefix (copied from apply.go context)
func nextPrefix(prefix []byte) []byte {
	out := make([]byte, len(prefix))
	copy(out, prefix)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 0xFF {
			out[i]++
			return out[:i+1]
		}
	}
	return nil // no upper bound if all 0xFF
}
