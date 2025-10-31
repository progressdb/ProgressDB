package frontend

import (
	"encoding/json"

	"progressdb/pkg/models"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/router/common"
	"progressdb/pkg/store/db/indexdb"
	message_store "progressdb/pkg/store/features/messages"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/pagination"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	author, tr, ok := common.SetupReadHandler(ctx, "read_threads_list")
	if !ok {
		return
	}
	defer tr.Finish()

	qp := pagination.ParsePaginationRequest(ctx)

	tr.Mark("get_user_threads")
	threadIDs, paginationResp, err := indexdb.GetUserThreadsCursor(author, qp.Cursor, qp.Limit)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tr.Mark("fetch_threads")
	out := make([]models.Thread, 0, len(threadIDs))
	for _, threadKey := range threadIDs {
		threadStr, err := thread_store.GetThread(threadKey)
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
	_ = router.WriteJSON(ctx, ThreadsListResponse{Threads: out, Pagination: paginationResp})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	author, tr, ok := common.SetupReadHandler(ctx, "read_thread_item")
	if !ok {
		return
	}
	defer tr.Finish()

	threadKey, valid := common.ValidatePathParam(ctx, "threadKey")
	if !valid {
		return
	}

	tr.Mark("validate_thread")
	thread, validationErr := common.ValidateReadThread(threadKey, author, true)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("write_response")
	_ = router.WriteJSON(ctx, ThreadResponse{Thread: *thread})
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	author, tr, ok := common.SetupReadHandler(ctx, "read_thread_messages")
	if !ok {
		return
	}
	defer tr.Finish()

	qp := pagination.ParsePaginationRequest(ctx)

	tr.Mark("validate_thread")
	threadKey, valid := common.ValidatePathParam(ctx, "threadKey")
	if !valid {
		return
	}
	_, validationErr := common.ValidateReadThread(threadKey, author, false)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("get_thread_indexes")
	threadIndexes, err := indexdb.GetThreadMessageIndexes(threadKey)
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
	author, tr, ok := common.SetupReadHandler(ctx, "read_thread_message")
	if !ok {
		return
	}
	defer tr.Finish()

	messageKey, valid := common.ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	threadKey := common.PathParam(ctx, "threadKey")

	tr.Mark("validate_thread")
	_, validationErr := common.ValidateReadThread(threadKey, author, false)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_message")
	message, validationErr := common.ValidateReadMessage(messageKey, author, true)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_thread_parent")
	if relErr := common.ValidateMessageThreadRelationship(message, threadKey); relErr != nil {
		common.WriteValidationError(ctx, relErr)
		return
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, MessageResponse{Message: *message})
}
