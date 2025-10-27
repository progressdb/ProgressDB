package admin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/routes/common"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/messages"
)

func AdminHealth(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	_, _ = ctx.WriteString(`{"status":"ok","service":"progressdb"}`)
}

func AdminStats(ctx *fasthttp.RequestCtx) {
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

func AdminListThreads(ctx *fasthttp.RequestCtx) {
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

func AdminListKeys(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	prefix := string(ctx.QueryArgs().Peek("prefix"))
	paginationReq := common.ParsePaginationRequest(ctx)

	result, err := listKeysByPrefixPaginated(prefix, paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func AdminGetKey(ctx *fasthttp.RequestCtx) {
	keyEnc, ok := extractParamOrFail(ctx, "key", "missing key")
	if !ok {
		return
	}
	key, err := url.PathUnescape(keyEnc)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid key encoding")
		return
	}
	val, err := storedb.GetKey(key)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	ctx.Response.Header.Set("Content-Type", "application/octet-stream")
	_, _ = ctx.Write([]byte(val))
}

func AdminListUsers(ctx *fasthttp.RequestCtx) {
	paginationReq := common.ParsePaginationRequest(ctx)

	result, err := listUsersByPrefixPaginated("idx:U:", paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func AdminListUserThreads(ctx *fasthttp.RequestCtx) {
	userID, ok := extractParamOrFail(ctx, "userId", "missing userId")
	if !ok {
		return
	}

	paginationReq := common.ParsePaginationRequest(ctx)

	result, err := listThreadsForUser(userID, paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func AdminListThreadMessages(ctx *fasthttp.RequestCtx) {
	threadID, ok := extractParamOrFail(ctx, "threadId", "missing threadId")
	if !ok {
		return
	}

	paginationReq := common.ParsePaginationRequest(ctx)

	result, err := listMessagesForThread(threadID, paginationReq)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, result)
}

func AdminGetThreadMessage(ctx *fasthttp.RequestCtx) {
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

func listKeysByPrefixPaginated(prefix string, paginationReq *common.PaginationRequest) (*DashboardKeysResult, error) {
	keys, nextCursor, hasMore, err := storedb.ListKeys(prefix, paginationReq.Limit, paginationReq.Cursor)
	if err != nil {
		return nil, err
	}

	return &DashboardKeysResult{
		Keys:       keys,
		Pagination: common.NewPaginationResponse(paginationReq.Limit, hasMore, nextCursor, len(keys)),
	}, nil
}

func listUsersByPrefixPaginated(prefix string, paginationReq *common.PaginationRequest) (*DashboardUsersResult, error) {
	relKeys, err := index.ListKeys(keys.UserThreadsRelPrefix)
	if err != nil {
		return nil, err
	}

	logger.Info("admin_list_users_debug", "prefix", keys.UserThreadsRelPrefix, "total_keys_found", len(relKeys), "sample_keys", func() []string {
		if len(relKeys) > 5 {
			return relKeys[:5]
		}
		return relKeys
	}())

	userSet := make(map[string]struct{})
	for _, key := range relKeys {
		parts := strings.Split(key, ":")
		if len(parts) >= 4 && parts[0] == "rel" && parts[1] == "u" && parts[3] == "t" {
			userSet[parts[2]] = struct{}{}
		}
	}

	allUsers := make([]string, 0, len(userSet))
	for user := range userSet {
		allUsers = append(allUsers, user)
	}
	sort.Strings(allUsers)

	start := 0
	if paginationReq.Cursor != "" {
		for i, userID := range allUsers {
			if userID == paginationReq.Cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + paginationReq.Limit
	if end > len(allUsers) {
		end = len(allUsers)
	}

	if start >= len(allUsers) {
		return &DashboardUsersResult{
			Users:      []string{},
			Pagination: common.NewPaginationResponse(paginationReq.Limit, false, "", 0),
		}, nil
	}

	pagedUsers := allUsers[start:end]
	nextCursor := ""
	if end < len(allUsers) {
		nextCursor = pagedUsers[len(pagedUsers)-1]
	}

	return &DashboardUsersResult{
		Users:      pagedUsers,
		Pagination: common.NewPaginationResponse(paginationReq.Limit, end < len(allUsers), nextCursor, len(pagedUsers)),
	}, nil
}

func listThreadsForUser(userID string, paginationReq *common.PaginationRequest) (*DashboardThreadsResult, error) {
	relKeys, err := index.ListKeys(keys.GenUserThreadRelPrefix(userID))
	if err != nil {
		return nil, fmt.Errorf("list user threads: %w", err)
	}

	allThreadIDs := make([]string, 0, len(relKeys))
	for _, key := range relKeys {
		parts := strings.Split(key, ":")
		if len(parts) >= 4 && parts[0] == "rel" && parts[1] == "u" && parts[2] == userID && parts[3] == "t" {
			allThreadIDs = append(allThreadIDs, parts[4])
		}
	}

	if len(allThreadIDs) == 0 {
		return &DashboardThreadsResult{
			Threads:    []json.RawMessage{},
			Pagination: common.NewPaginationResponse(paginationReq.Limit, false, "", 0),
		}, nil
	}

	start := 0
	if paginationReq.Cursor != "" {
		for i, tid := range allThreadIDs {
			if tid == paginationReq.Cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + paginationReq.Limit
	if end > len(allThreadIDs) {
		end = len(allThreadIDs)
	}

	pageThreads := allThreadIDs[start:end]
	hasMore := end < len(allThreadIDs)
	nextCursor := ""
	if hasMore && len(pageThreads) > 0 {
		nextCursor = pageThreads[len(pageThreads)-1]
	}

	threads := make([]json.RawMessage, 0, len(pageThreads))
	for _, threadID := range pageThreads {
		data, err := storedb.GetKey(keys.GenThreadKey(threadID))
		if err != nil {
			continue
		}
		threads = append(threads, json.RawMessage(data))
	}

	return &DashboardThreadsResult{
		Threads:    threads,
		Pagination: common.NewPaginationResponse(paginationReq.Limit, hasMore, nextCursor, len(threads)),
	}, nil
}

func listMessagesForThread(threadID string, paginationReq *common.PaginationRequest) (*DashboardMessagesResult, error) {
	reqCursor := models.ReadRequestCursorInfo{
		Cursor: paginationReq.Cursor,
		Limit:  paginationReq.Limit,
	}
	messages, respCursor, err := messages.ListMessages(threadID, reqCursor)
	if err != nil {
		return nil, err
	}

	return &DashboardMessagesResult{
		Messages:   router.ToRawMessages(messages),
		Pagination: common.NewPaginationResponse(paginationReq.Limit, respCursor.HasMore, respCursor.Cursor, len(messages)),
	}, nil
}

func listAllThreads() ([]string, error) {
	allKeys, _, _, err := storedb.ListKeys(keys.GenThreadMetadataPrefix(), 10000, "")
	if err != nil {
		return nil, err
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
