package admin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"

	"github.com/cockroachdb/pebble"
	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/utils"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/iterator/admin/ki"
	"progressdb/pkg/store/iterator/admin/mi"
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

	// Use admin key iterator to get all thread metadata keys
	keyIter := ki.NewKeyIterator(storedb.Client)

	// Get all thread keys with large limit
	req := pagination.PaginationRequest{Limit: 10000}
	threadKeys, _, err := keyIter.ExecuteKeyQuery(keys.GenThreadMetadataPrefix(), req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// Count threads and messages
	var threadCount int
	var msgCount int64

	for _, key := range threadKeys {
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
				indexes, err := indexdb.GetThreadMessageIndexData(th.Key)
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

	// Use admin key iterator to get all thread metadata keys
	keyIter := ki.NewKeyIterator(storedb.Client)

	// Get all thread keys with large limit
	req := pagination.PaginationRequest{Limit: 10000}
	threadKeys, _, err := keyIter.ExecuteKeyQuery(keys.GenThreadMetadataPrefix(), req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// Filter only thread keys and get their values
	var threads []string
	for _, key := range threadKeys {
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
	prefix := utils.GetQuery(ctx, "prefix")
	store := utils.GetQuery(ctx, "store")
	if store == "" {
		store = "main"
	}
	paginationReq := utils.ParsePaginationRequest(ctx)

	// Use admin key iterator for proper pagination
	var keys []string
	var paginationResp pagination.PaginationResponse
	var err error

	if store == "index" {
		keyIter := ki.NewKeyIterator(indexdb.Client)
		keys, paginationResp, err = keyIter.ExecuteKeyQuery(prefix, paginationReq)
	} else {
		keyIter := ki.NewKeyIterator(storedb.Client)
		keys, paginationResp, err = keyIter.ExecuteKeyQuery(prefix, paginationReq)
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
		Users: allUsers,
		Pagination: pagination.PaginationResponse{
			Count: len(allUsers),
			Total: len(allUsers),
		},
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
		Threads: threadKeys,
		Pagination: pagination.PaginationResponse{
			Count: len(threadKeys),
			Total: len(threadKeys),
		},
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

	// Use admin message iterator for proper pagination
	msgIter := mi.NewMessageIterator(storedb.Client)

	// Parse pagination request from query params
	paginationReq := utils.ParsePaginationRequest(ctx)
	if paginationReq.Limit == 0 {
		paginationReq.Limit = 100 // default
	}

	// Execute message query for this thread
	msgKeys, paginationResp, err := msgIter.ExecuteMessageQuery(parsedThread.ThreadKey, paginationReq)
	if err != nil {
		logger.Error("ListThreadMessages: failed to execute message query:", err)
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	logger.Info("ListThreadMessages: found", len(msgKeys), "messages for thread", threadKey)

	result := &DashboardMessagesResult{
		Messages:   msgKeys,
		Pagination: paginationResp,
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
