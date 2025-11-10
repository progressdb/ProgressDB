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
	author, _, ok := router.SetupReadHandler(ctx, "read_threads_list")
	if !ok {
		return
	}

	req := utils.ParsePaginationRequest(ctx)

	if err := utils.ValidatePaginationRequest(req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid pagination: %v", err))
		return
	}

	threadIter := ti.NewThreadIterator(indexdb.Client)
	threadKeys, paginationResp, err := threadIter.ExecuteThreadQuery(author, req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to read threads: %v", err))
		return
	}

	fetcher := ti.NewThreadFetcher()
	threads, err := fetcher.FetchThreads(threadKeys, author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to fetch threads: %v", err))
		return
	}

	sorter := ti.NewThreadSorter()
	threads = sorter.SortThreads(threads, req.SortBy)

	_ = router.WriteJSON(ctx, ThreadsListResponse{Threads: threads, Pagination: &paginationResp})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	author, _, ok := router.SetupReadHandler(ctx, "read_thread_item")
	if !ok {
		return
	}

	threadKey, valid := router.ValidatePathParam(ctx, "threadKey")
	if !valid {
		return
	}

	// check access via ownership or participation
	hasOwnership, err := indexdb.DoesUserOwnThread(author, threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to check thread ownership: %v", err))
		return
	}

	hasParticipation, err := indexdb.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to check thread participation: %v", err))
		return
	}

	if !hasOwnership && !hasParticipation {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "access denied")
		return
	}

	thread, validationErr := router.ValidateReadThread(threadKey, author, true)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	_ = router.WriteJSON(ctx, ThreadResponse{Thread: *thread})
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	author, _, ok := router.SetupReadHandler(ctx, "read_thread_messages")
	if !ok {
		return
	}

	threadKey, valid := router.ValidatePathParam(ctx, "threadKey")
	if !valid {
		return
	}

	// check access via ownership or participation
	hasOwnership, err := indexdb.DoesUserOwnThread(author, threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to check thread ownership: %v", err))
		return
	}

	hasParticipation, err := indexdb.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to check thread participation: %v", err))
		return
	}

	if !hasOwnership && !hasParticipation {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "access denied")
		return
	}

	_, validationErr := router.ValidateReadThread(threadKey, author, false)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	req := utils.ParsePaginationRequest(ctx)

	if err := utils.ValidatePaginationRequest(req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid pagination: %v", err))
		return
	}

	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to generate message prefix: %v", err))
		return
	}

	keyIter := ki.NewKeyIterator(storedb.Client)
	messageKeys, paginationResp, err := keyIter.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to read messages: %v", err))
		return
	}

	fetcher := mi.NewMessageFetcher()
	messages, err := fetcher.FetchMessages(messageKeys)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to fetch messages: %v", err))
		return
	}

	sorter := mi.NewMessageSorter()
	messages = sorter.SortMessages(messages, req.SortBy)

	_ = router.WriteJSON(ctx, MessagesListResponse{Thread: threadKey, Messages: messages, Pagination: &paginationResp})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	author, _, ok := router.SetupReadHandler(ctx, "read_thread_message")
	if !ok {
		return
	}

	messageKey, valid := router.ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	threadKey := router.PathParam(ctx, "threadKey")

	// check access via ownership or participation
	hasOwnership, err := indexdb.DoesUserOwnThread(author, threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to check thread ownership: %v", err))
		return
	}

	hasParticipation, err := indexdb.DoesThreadHaveUser(threadKey, author)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, fmt.Sprintf("failed to check thread participation: %v", err))
		return
	}

	if !hasOwnership && !hasParticipation {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "access denied")
		return
	}

	_, validationErr := router.ValidateReadThread(threadKey, author, false)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	// For individual message access, allow thread participants to read any message in the thread
	message, validationErr := router.ValidateReadMessage(messageKey, author, false)
	if validationErr != nil {
		router.WriteValidationError(ctx, validationErr)
		return
	}

	if relErr := router.ValidateMessageThreadRelationship(message, threadKey); relErr != nil {
		router.WriteValidationError(ctx, relErr)
		return
	}

	_ = router.WriteJSON(ctx, MessageResponse{Message: *message})
}
