package admin

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/features/messages"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"progressdb/pkg/state/logger"
)

func Health(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	_, _ = ctx.WriteString(`{"status":"ok","service":"progressdb"}`)
}

func Stats(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	threadList, _ := listAllThreads()
	var msgCount int64
	for _, raw := range threadList {
		var th models.Thread
		if err := json.Unmarshal([]byte(raw), &th); err != nil {
			continue
		}
		indexes, err := index.GetThreadMessageIndexes(th.Key)
		if err != nil {
			continue
		}
		msgCount += int64(indexes.End)
	}
	_ = router.WriteJSON(ctx, struct {
		Threads  int   `json:"threads"`
		Messages int64 `json:"messages"`
	}{Threads: len(threadList), Messages: msgCount})
}

func ListThreads(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	vals, err := listAllThreads()
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, struct {
		Threads []json.RawMessage `json:"threads"`
	}{Threads: router.ToRawMessages(vals)})
}

func ListKeys(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	prefix := string(ctx.QueryArgs().Peek("prefix"))
	store := string(ctx.QueryArgs().Peek("store"))
	if store == "" {
		store = "main"
	}
	paginationReq := pagination.ParsePaginationRequest(ctx)

	result, err := listKeysByPrefixPaginated(prefix, store, paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
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
	storeParam := string(ctx.QueryArgs().Peek("store"))
	var val string
	switch storeParam {
	case "index":
		val, err = index.GetKey(key)
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
	paginationReq := pagination.ParsePaginationRequest(ctx)

	result, err := listUsersByPrefixPaginated(keys.UserThreadsRelPrefix, paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func ListUserThreads(ctx *fasthttp.RequestCtx) {
	userID, ok := extractParamOrFail(ctx, "userId", "missing userId")
	if !ok {
		return
	}

	paginationReq := pagination.ParsePaginationRequest(ctx)

	result, err := listThreadsForUser(userID, paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func ListThreadMessages(ctx *fasthttp.RequestCtx) {
	threadKey, ok := extractParamOrFail(ctx, "threadKey", "missing threadKey")
	if !ok {
		return
	}

	paginationReq := pagination.ParsePaginationRequest(ctx)

	result, err := listMessagesForThread(threadKey, paginationReq)
	if err != nil {
		logger.Error("ListThreadMessages: %v", err)
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func GetThreadMessage(ctx *fasthttp.RequestCtx) {
	messageID, ok := extractParamOrFail(ctx, "messageId", "missing messageId")
	if !ok {
		return
	}

	msg, err := messages.GetLatestMessage(messageID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

	ctx.Response.Header.Set("Content-Type", "application/json")
	_, _ = ctx.Write([]byte(msg))
}

// helpers
func listKeysByPrefixPaginated(prefix string, store string, paginationReq *pagination.PaginationRequest) (*DashboardKeysResult, error) {
	var keys []string
	var paginationResp *pagination.PaginationResponse
	var err error

	if store == "index" {
		keys, paginationResp, err = index.ListKeysWithPrefixPaginated(prefix, paginationReq)
	} else {
		keys, paginationResp, err = storedb.ListKeysWithPrefixPaginated(prefix, paginationReq)
	}

	if err != nil {
		return nil, err
	}

	return &DashboardKeysResult{
		Keys:       keys,
		Pagination: paginationResp,
	}, nil
}

// Util function for paginating a slice of items
type paginationUtilResult struct {
	Items    []string
	Response *pagination.PaginationResponse
}

func newPaginationResponseUtil(limit int, items []string, total int) paginationUtilResult {
	hasMore := false
	nextCursor := ""
	page := items
	if len(items) > limit {
		hasMore = true
		page = items[:limit]
		nextCursor = page[len(page)-1]
	}
	return paginationUtilResult{
		Items: page,
		Response: pagination.NewPaginationResponse(
			limit,
			hasMore,
			nextCursor,
			len(page),
			total,
		),
	}
}

func listUsersByPrefixPaginated(prefix string, _ *pagination.PaginationRequest) (*DashboardUsersResult, error) {
	// Open a raw Pebble iterator over the index database, using prefix and its next lexicographical key for bounds
	lowerBound := []byte(prefix)
	upperBound := nextPrefix(lowerBound)

	iter, err := index.IndexDB.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	userSet := make(map[string]struct{})

	for valid := iter.First(); valid; valid = iter.Next() {
		keyStr := string(iter.Key())
		parts := strings.Split(keyStr, ":")
		if len(parts) >= 4 && parts[0] == "rel" && parts[1] == "u" && parts[3] == "t" {
			userSet[parts[2]] = struct{}{}
		}
	}

	// Convert userSet keys to sorted slice for output
	allUsers := make([]string, 0, len(userSet))
	for user := range userSet {
		allUsers = append(allUsers, user)
	}
	sort.Strings(allUsers)

	return &DashboardUsersResult{
		Users:      allUsers,
		Pagination: nil,
	}, nil
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

func listThreadsForUser(userID string, _ *pagination.PaginationRequest) (*DashboardThreadsResult, error) {
	// Validate userID using @validate.go
	if err := keys.ValidateUserID(userID); err != nil {
		return nil, err
	}

	prefix := keys.GenUserThreadRelPrefix(userID)
	lowerBound := []byte(prefix)
	upperBound := nextPrefix(lowerBound)

	iter, err := index.IndexDB.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var threadKeys []string
	count := 0
	const maxThreads = 100

	for valid := iter.First(); valid && count < maxThreads; valid = iter.Next() {
		keyStr := string(iter.Key())

		// Validate the relationship key
		if err := keys.ValidateUserOwnsThreadKey(keyStr); err != nil {
			continue
		}

		// Extract threadID from the validated key
		parsedKey, err := keys.ParseUserOwnsThread(keyStr)
		if err != nil {
			// log the error
			logger.Error("failed to parse user owns thread key %q: %v", keyStr, err)
			continue
		}

		// Validate threadKey
		if err := keys.ValidateThreadKey(parsedKey.ThreadKey); err != nil {
			// log the error
			logger.Error("invalid thread key in user thread relation %q: %v", parsedKey.ThreadKey, err)
			continue
		}

		threadKeys = append(threadKeys, parsedKey.ThreadKey)
		count++
	}

	return &DashboardThreadsResult{
		Threads:    threadKeys,
		Pagination: nil,
	}, nil
}

func listMessagesForThread(threadKey string, paginationReq *pagination.PaginationRequest) (*DashboardMessagesResult, error) {
	logger.Info("listMessagesForThread: listing messages for thread", threadKey)

	// Validate thread key
	if err := keys.ValidateThreadKey(threadKey); err != nil {
		logger.Error("listMessagesForThread: invalid thread key", threadKey, err)
		return nil, err
	}

	// Generate prefix for messages of this thread
	prefix := keys.GenAllThreadMessagesPrefix(threadKey)
	lowerBound := []byte(prefix)
	upperBound := nextPrefix(lowerBound)

	logger.Debug("listMessagesForThread: using range lowerBound=", string(lowerBound), "upperBound=", string(upperBound))

	iter, err := index.IndexDB.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		logger.Error("listMessagesForThread: failed to create iterator:", err)
		return nil, err
	}
	defer iter.Close()

	var msgKeys []string
	count := 0
	limit := 100 // default

	for valid := iter.First(); valid && count < limit; valid = iter.Next() {
		messagekey := string(iter.Key())
		// Only include direct message keys for the thread
		if err := keys.ValidateMessageKey(messagekey); err != nil {
			logger.Debug("listMessagesForThread: skipping invalid message key", messagekey, err)
			continue
		}
		logger.Debug("listMessagesForThread: messagekey", messagekey)
		msgKeys = append(msgKeys, messagekey)
		count++
	}
	logger.Info("listMessagesForThread: found", count, "messages for thread", threadKey)

	return &DashboardMessagesResult{
		Messages:   msgKeys,
		Pagination: pagination.NewPaginationResponse(limit, count == limit, "", count, count), // Cursor-based pagination can be improved if needed
	}, nil
}

func listAllThreads() ([]string, error) {
	var allKeys []string
	cursor := ""
	for {
		keys, resp, err := storedb.ListKeysWithPrefixPaginated(keys.GenThreadMetadataPrefix(), &pagination.PaginationRequest{Limit: 100, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		allKeys = append(allKeys, keys...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}

	var threadKeys []string
	for _, key := range allKeys {
		if strings.Count(key, ":") == 1 && strings.HasPrefix(key, "t:") {
			threadKeys = append(threadKeys, key)
		}
	}

	var threads []string
	for _, key := range threadKeys {
		val, err := storedb.GetKey(key)
		if err != nil {
			continue
		}
		threads = append(threads, val)
	}

	return threads, nil
}
