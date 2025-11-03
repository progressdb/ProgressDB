package frontend

import (
	"fmt"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/utils"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/iterator/frontend/mi"
	"progressdb/pkg/store/iterator/frontend/ti"
	"progressdb/pkg/store/keys"
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
	threadIter := ti.NewThreadIterator(indexdb.Client)
	threadKeys, paginationResp, err := threadIter.ExecuteThreadQuery(author, req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to read threads: %v", err))
		return
	}

	tr.Mark("fetch_threads")
	fetcher := ti.NewThreadFetcher()
	threads, err := fetcher.FetchThreads(threadKeys, author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to fetch threads: %v", err))
		return
	}

	tr.Mark("sort_threads")
	sorter := ti.NewThreadSorter()
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

	tr.Mark("parse_pagination")
	req := utils.ParsePaginationRequest(ctx)

	// Validate request
	if err := utils.ValidatePaginationRequest(req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid pagination: %v", err))
		return
	}

	tr.Mark("query_messages")
	// Use proper keys method to generate message prefix
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to generate message prefix: %v", err))
		return
	}

	// Debug: Log the prefix being used
	fmt.Printf("[DEBUG] Generated message prefix: %q for threadKey: %q\n", messagePrefix, threadKey)

	keyIter := ki.NewKeyIterator(storedb.Client)
	messageKeys, paginationResp, err := keyIter.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to read messages: %v", err))
		return
	}

	// Debug: Log what we got from key iterator
	if len(messageKeys) == 0 {
		fmt.Printf("[DEBUG] No message keys found - checking if prefix is correct\n")
	}

	tr.Mark("fetch_messages")
	fetcher := mi.NewMessageFetcher()
	messages, err := fetcher.FetchMessages(messageKeys)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to fetch messages: %v", err))
		return
	}

	tr.Mark("sort_messages")
	sorter := mi.NewMessageSorter()
	messages = sorter.SortMessages(messages, req.SortBy, req.OrderBy)

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, MessagesListResponse{Thread: threadKey, Messages: messages, Pagination: &paginationResp})
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
