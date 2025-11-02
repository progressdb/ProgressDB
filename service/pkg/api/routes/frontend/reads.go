package frontend

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/utils"
	"progressdb/pkg/store/db/indexdb"
	message_store "progressdb/pkg/store/features/messages"

	"progressdb/pkg/store/iterator/thread"
	"progressdb/pkg/store/pagination"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	author, tr, ok := router.SetupReadHandler(ctx, "read_threads_list")
	if !ok {
		return
	}
	defer tr.Finish()

	tr.Mark("parse_pagination")
	req := utils.ParsePaginationRequest(ctx)

	if err := utils.ValidatePaginationRequest(req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid pagination: %v", err))
		return
	}

	tr.Mark("query_threads")
	threadIter := thread.NewThreadIterator(indexdb.Client)
	threadKeys, paginationResp, err := threadIter.ExecuteThreadQuery(author, req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to read threads: %v", err))
		return
	}

	tr.Mark("fetch_threads")
	fetcher := thread.NewThreadFetcher()
	threads, err := fetcher.FetchThreads(threadKeys, author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to fetch threads: %v", err))
		return
	}

	tr.Mark("sort_threads")
	sorter := thread.NewThreadSorter()
	threads = sorter.SortThreads(threads, req.SortBy, req.OrderBy)

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, ThreadsListResponse{Threads: threads, Pagination: &paginationResp})
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

	// Parse new pagination parameters
	req := utils.ParsePaginationRequest(ctx)

	// Validate request
	if err := utils.ValidatePaginationRequest(req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid pagination: %v", err))
		return
	}

	// TODO: Implement message iterator with new pagination system
	// For now, use existing message store with converted parameters
	reqCursor := models.ReadRequestCursorInfo{
		Cursor: "", // New pagination doesn't use cursor
		Limit:  req.Limit,
	}
	rawMsgs, respCursor, err := message_store.ListMessages(threadKey, reqCursor)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	// Convert to new response format
	paginationResp := pagination.PaginationResponse{
		HasAfter: respCursor.HasMore,
		OrderBy:  req.OrderBy,
		Count:    len(rawMsgs),
		Total:    int(respCursor.TotalCount),
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
	_ = router.WriteJSON(ctx, MessagesListResponse{Thread: threadKey, Messages: msgs, Metadata: threadIndexes, Pagination: &paginationResp})
}

// readMessagesWithNewPagination uses the new bidirectional pagination system (POC)
func readMessagesWithNewPagination(ctx *fasthttp.RequestCtx, threadKey string) ([]string, pagination.PaginationResponse, error) {
	// POC implementation - for now, fall back to legacy
	return readMessagesWithLegacyPagination(ctx, threadKey)
}

// readMessagesWithLegacyPagination uses the original cursor-based system
func readMessagesWithLegacyPagination(ctx *fasthttp.RequestCtx, threadKey string) ([]string, pagination.PaginationResponse, error) {
	// Parse pagination
	qp := utils.ParsePaginationRequest(ctx)

	reqCursor := models.ReadRequestCursorInfo{
		Cursor: "", // Legacy cursor field doesn't exist in new struct
		Limit:  qp.Limit,
	}
	rawMsgs, respCursor, err := message_store.ListMessages(threadKey, reqCursor)
	if err != nil {
		return nil, pagination.PaginationResponse{}, err
	}

	// Convert to new response format
	response := pagination.PaginationResponse{
		HasAfter: respCursor.HasMore,
		OrderBy:  "asc", // Messages are typically read chronologically
		Count:    len(rawMsgs),
		Total:    int(respCursor.TotalCount),
	}

	return rawMsgs, response, nil
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
