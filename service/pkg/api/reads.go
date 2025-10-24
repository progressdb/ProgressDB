package api

import (
	"encoding/json"

	"progressdb/pkg/models"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/store/db/index"
	message_store "progressdb/pkg/store/messages"
	thread_store "progressdb/pkg/store/threads"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	author, tr, ok := SetupReadHandler(ctx, "read_threads_list")
	if !ok {
		return
	}
	defer tr.Finish()

	qp := ParseQueryParameters(ctx)

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
	pagination := PaginationMeta{
		Limit:      qp.Limit,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Count:      len(out),
	}
	_ = router.WriteJSON(ctx, ThreadsListResponse{Threads: out, Pagination: pagination})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	author, tr, ok := SetupReadHandler(ctx, "read_thread_item")
	if !ok {
		return
	}
	defer tr.Finish()

	id, valid := ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	tr.Mark("validate_thread")
	thread, validationErr := ValidateThread(id, author, true)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("write_response")
	_ = router.WriteJSON(ctx, ThreadResponse{Thread: *thread})
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	author, tr, ok := SetupReadHandler(ctx, "read_thread_messages")
	if !ok {
		return
	}
	defer tr.Finish()

	qp := ParseQueryParameters(ctx)

	tr.Mark("validate_thread")
	threadID, valid := ValidatePathParam(ctx, "threadID")
	if !valid {
		return
	}
	_, validationErr := ValidateThread(threadID, author, false)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
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
	pagination := PaginationMeta{
		Limit:      qp.Limit,
		HasMore:    respCursor.HasMore,
		NextCursor: respCursor.Cursor,
		Count:      len(msgs),
	}
	_ = router.WriteJSON(ctx, MessagesListResponse{Thread: threadID, Messages: msgs, Metadata: threadIndexes, Pagination: pagination})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	author, tr, ok := SetupReadHandler(ctx, "read_thread_message")
	if !ok {
		return
	}
	defer tr.Finish()

	messageID, valid := ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	threadID := pathParam(ctx, "threadID")

	tr.Mark("validate_thread")
	_, validationErr := ValidateThread(threadID, author, false)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_message")
	message, validationErr := ValidateMessage(messageID, author, true)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_thread_parent")
	if relErr := ValidateMessageThreadRelationship(message, threadID); relErr != nil {
		WriteValidationError(ctx, relErr)
		return
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, MessageResponse{Message: *message})
}

func ReadMessageReactions(ctx *fasthttp.RequestCtx) {
	author, tr, ok := SetupReadHandler(ctx, "read_message_reactions")
	if !ok {
		return
	}
	defer tr.Finish()

	messageID, valid := ValidatePathParam(ctx, "id")
	if !valid {
		return
	}

	threadID := pathParam(ctx, "threadID")

	tr.Mark("validate_thread")
	_, validationErr := ValidateThread(threadID, author, false)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("validate_message")
	message, validationErr := ValidateMessage(messageID, author, false)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	if relErr := ValidateMessageThreadRelationship(message, threadID); relErr != nil {
		WriteValidationError(ctx, relErr)
		return
	}

	tr.Mark("process_reactions")
	type reaction struct {
		ID       string `json:"id"`
		Reaction string `json:"reaction"`
	}

	out := make([]reaction, 0, len(message.Reactions))
	for k, v := range message.Reactions {
		out = append(out, reaction{ID: k, Reaction: v})
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, ReactionsResponse{ID: messageID, Reactions: out})
}
