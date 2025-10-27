package frontend

import (
	"encoding/json"

	"progressdb/pkg/models"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/routes/common"
	"progressdb/pkg/store/db/index"
	message_store "progressdb/pkg/store/messages"
	thread_store "progressdb/pkg/store/threads"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	author, tr, ok := common.SetupReadHandler(ctx, "read_threads_list")
	if !ok {
		return
	}
	defer tr.Finish()

	qp := common.ParsePaginationRequest(ctx)

	tr.Mark("get_user_threads")
	threadIDs, nextCursor, hasMore, err := index.GetUserThreadsCursor(author, qp.Cursor, qp.Limit)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tr.Mark("fetch_threads")
	out := make([]models.Thread, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		threadStr, err := thread_store.GetThread(threadID)
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
	pagination := common.NewPaginationResponse(qp.Limit, hasMore, nextCursor, len(out))
	_ = router.WriteJSON(ctx, ThreadsListResponse{Threads: out, Pagination: pagination})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	author, tr, ok := common.SetupReadHandler(ctx, "read_thread_item")
	if !ok {
		return
	}
	defer tr.Finish()

	id, valid := common.ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	tr.Mark("validate_thread")
	thread, validationErr := common.ValidateReadThread(id, author, true)
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

	qp := common.ParsePaginationRequest(ctx)

	tr.Mark("validate_thread")
	threadID, valid := common.ValidatePathParam(ctx, "threadID")
	if !valid {
		return
	}
	_, validationErr := common.ValidateReadThread(threadID, author, false)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("get_thread_indexes")
	threadIndexes, err := index.GetThreadMessageIndexes(threadID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tr.Mark("list_messages")
	reqCursor := models.ReadRequestCursorInfo{
		Cursor: qp.Cursor,
		Limit:  qp.Limit,
	}
	rawMsgs, respCursor, err := message_store.ListMessages(threadID, reqCursor)
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
	pagination := common.NewPaginationResponse(qp.Limit, respCursor.HasMore, respCursor.Cursor, len(msgs))
	_ = router.WriteJSON(ctx, MessagesListResponse{Thread: threadID, Messages: msgs, Metadata: threadIndexes, Pagination: pagination})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	author, tr, ok := common.SetupReadHandler(ctx, "read_thread_message")
	if !ok {
		return
	}
	defer tr.Finish()

	messageID, valid := common.ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	threadID := common.PathParam(ctx, "threadID")

	tr.Mark("validate_thread")
	_, validationErr := common.ValidateReadThread(threadID, author, false)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_message")
	message, validationErr := common.ValidateReadMessage(messageID, author, true)
	if validationErr != nil {
		common.WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_thread_parent")
	if relErr := common.ValidateMessageThreadRelationship(message, threadID); relErr != nil {
		common.WriteValidationError(ctx, relErr)
		return
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, MessageResponse{Message: *message})
}
