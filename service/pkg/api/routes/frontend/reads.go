package frontend

import (
	"encoding/json"
	"fmt"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/store/keys"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/store/db/indexdb"
	message_store "progressdb/pkg/store/features/messages"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/pagination"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	author, tr, ok := router.SetupReadHandler(ctx, "read_threads_list")
	if !ok {
		return
	}
	defer tr.Finish()

	// parse paging req for defaults
	qp := pagination.ParsePaginationRequest(ctx)

	tr.Mark("get_user_threads")

	// Decode cursor to get starting point
	cursorPayload, err := pagination.DecodeCursor(qp.Cursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid cursor: %v", err))
		return
	}

	// Generate user thread relationship prefix
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to generate prefix: %v", err))
		return
	}

	// Create iterator
	iter, err := indexdb.Client.NewIter(nil)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to create iterator: %v", err))
		return
	}
	defer iter.Close()

	var threadKeys []string
	var LastListItemKey string
	count := 0

	// Determine starting point
	var startKey []byte
	if cursorPayload.LastListItemKey != "" {
		startKey = []byte(cursorPayload.LastListItemKey)
	} else {
		startKey = []byte(userThreadPrefix)
	}

	// Iterate through user thread relationships
	for ok := iter.SeekGE(startKey); ok && iter.Valid(); ok = iter.Next() {
		key := string(iter.Key())

		// Stop if we're past the user thread prefix
		if !strings.HasPrefix(key, userThreadPrefix) {
			break
		}

		// Skip the cursor key itself if we're starting from a cursor
		if cursorPayload.LastListItemKey != "" && key == cursorPayload.LastListItemKey {
			continue
		}

		// Parse the user owns thread relationship key
		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			continue
		}

		threadKeys = append(threadKeys, parsed.ThreadKey)
		LastListItemKey = key
		count++

		if count >= qp.Limit {
			break
		}
	}

	// Check if there are more results
	hasMore := iter.Valid() && strings.HasPrefix(string(iter.Key()), userThreadPrefix)

	tr.Mark("fetch_threads")
	out := make([]models.Thread, 0, len(threadKeys))
	for _, threadKey := range threadKeys {
		threadStr, err := thread_store.GetThreadData(threadKey)
		if err != nil {
			continue
		}
		var thread models.Thread
		if err := json.Unmarshal([]byte(threadStr), &thread); err != nil {
			continue
		}
		if thread.Author != author {
			continue
		}
		out = append(out, thread)
	}

	tr.Mark("encode_response")

	// Encode next cursor
	var nextCursor string
	if hasMore && LastListItemKey != "" {
		nextCursor = pagination.EncodeCursor(pagination.CursorPayload{
			LastListItemKey: LastListItemKey,
		})
	}

	paginationResp := pagination.NewPaginationResponse(qp.Limit, hasMore, nextCursor, len(out), 0)
	_ = router.WriteJSON(ctx, ThreadsListResponse{Threads: out, Pagination: paginationResp})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	author, tr, ok := router.SetupReadHandler(ctx, "read_thread_item")
	if !ok {
		return
	}
	defer tr.Finish()

	threadKey, valid := router.ValidatePathParam(ctx, "threadKey")
	if !valid {
		return
	}

	tr.Mark("validate_thread")
	thread, validationErr := router.ValidateReadThread(threadKey, author, true)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("write_response")
	_ = router.WriteJSON(ctx, ThreadResponse{Thread: *thread})
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	author, tr, ok := router.SetupReadHandler(ctx, "read_thread_messages")
	if !ok {
		return
	}
	defer tr.Finish()

	qp := pagination.ParsePaginationRequest(ctx)

	tr.Mark("validate_thread")
	threadKey, valid := router.ValidatePathParam(ctx, "threadKey")
	if !valid {
		return
	}
	_, validationErr := router.ValidateReadThread(threadKey, author, false)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("get_thread_indexes")
	threadIndexes, err := indexdb.GetThreadMessageIndexData(threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tr.Mark("list_messages")
	reqCursor := models.ReadRequestCursorInfo{
		Cursor: qp.Cursor,
		Limit:  qp.Limit,
	}
	rawMsgs, respCursor, err := message_store.ListMessages(threadKey, reqCursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tr.Mark("process_messages")
	msgs := make([]models.Message, 0, len(rawMsgs))
	for _, raw := range rawMsgs {
		var msg models.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}

	tr.Mark("encode_response")
	paginationResp := pagination.NewPaginationResponse(qp.Limit, respCursor.HasMore, respCursor.Cursor, len(msgs), int(respCursor.TotalCount))
	_ = router.WriteJSON(ctx, MessagesListResponse{Thread: threadKey, Messages: msgs, Metadata: threadIndexes, Pagination: paginationResp})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	author, tr, ok := router.SetupReadHandler(ctx, "read_thread_message")
	if !ok {
		return
	}
	defer tr.Finish()

	messageKey, valid := router.ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	threadKey := router.PathParam(ctx, "threadKey")

	tr.Mark("validate_thread")
	_, validationErr := router.ValidateReadThread(threadKey, author, false)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_message")
	message, validationErr := router.ValidateReadMessage(messageKey, author, true)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_thread_parent")
	if relErr := router.ValidateMessageThreadRelationship(message, threadKey); relErr != nil {
		router.WriteValidationError(ctx, relErr)
		return
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, MessageResponse{Message: *message})
}
