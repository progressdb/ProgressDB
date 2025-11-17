package frontend

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/tracking"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/timeutil"
)

// Standardized error handling for queue operations
func handleQueueError(ctx *fasthttp.RequestCtx, err error) {
	if err == nil {
		return
	}
	switch err {
	case queue.ErrQueueFull:
		router.WriteJSONError(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
	case queue.ErrQueueClosed:
		router.WriteJSONError(ctx, fasthttp.StatusServiceUnavailable, "server shutting down")
	default:
		logger.Error("enqueue_failed", "error", err)
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
	}
}

// thread management
func EnqueueCreateThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// sync
	reqtime := timeutil.Now().UnixNano()

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}

	// parse
	payload, ok := router.ExtractPayloadOrFail(ctx)
	if !ok {
		return
	}
	var th models.Thread
	if err := json.Unmarshal(payload, &th); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread payload")
		return
	}
	metadata := router.NewRequestMetadata(ctx, author)

	// sync
	threadKey := keys.GenThreadPrvKey(fmt.Sprintf("%d", reqtime))
	th.Key = threadKey
	th.Author = author
	th.CreatedTS = reqtime
	th.UpdatedTS = reqtime

	// validate
	if err := router.ValidateAllFieldsNonEmpty(&th); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := queue.GlobalIngestQueue.Enqueue(&types.QueueOp{
		Handler: types.HandlerThreadCreate,
		Payload: &th,
		TS:      reqtime,
		Extras: types.RequestMetadata{
			ApiRole: metadata.ApiRole,
			UserID:  metadata.UserID,
			ReqID:   metadata.ReqID,
			ReqIP:   metadata.ReqIP,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}

	// Track thread creation in-flight
	tracking.GlobalInflightTracker.Add(threadKey)

	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = router.WriteJSON(ctx, map[string]string{"key": threadKey})
}

func EnqueueUpdateThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// sync
	reqtime := timeutil.Now().UnixNano()

	// extract
	threadKey, ok := router.ExtractParamOrFail(ctx, "threadKey", "thread id missing")
	if !ok {
		return
	}

	// resolve provisional keys to final keys
	resolvedThreadKey, err := tracking.GlobalKeyMapper.ResolveKeyOrWait(threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, "thread not found")
		return
	}

	// validate
	if err := router.ValidateThreadKey(resolvedThreadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// validate - del status
	if err := router.ValidateThreadNotDeleted(resolvedThreadKey); err != nil {
		router.HandleDeletedError(ctx, err)
		return
	}

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}

	// parse
	payload, ok := router.ExtractPayloadOrFail(ctx)
	if !ok {
		return
	}

	var update models.ThreadUpdatePartial
	if err := json.Unmarshal(payload, &update); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread update payload")
		return
	}
	metadata := router.NewRequestMetadata(ctx, author)

	// sync
	update.Key = resolvedThreadKey
	update.UpdatedTS = reqtime

	// validate
	if err := router.ValidateAllFieldsNonEmpty(&update); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := queue.GlobalIngestQueue.Enqueue(&types.QueueOp{
		Handler: types.HandlerThreadUpdate,
		Payload: &update,
		TS:      reqtime,
		Extras: types.RequestMetadata{
			ApiRole: metadata.ApiRole,
			UserID:  metadata.UserID,
			ReqID:   metadata.ReqID,
			ReqIP:   metadata.ReqIP,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = router.WriteJSON(ctx, map[string]string{"key": resolvedThreadKey})
}

func EnqueueDeleteThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// sync
	reqtime := timeutil.Now().UnixNano()

	threadKey, ok := router.ExtractParamOrFail(ctx, "threadKey", "thread id missing")
	if !ok {
		return
	}

	// resolve provisional keys to final keys
	resolvedThreadKey, err := tracking.GlobalKeyMapper.ResolveKeyOrWait(threadKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, "thread not found")
		return
	}

	// validate
	if err := router.ValidateThreadKey(resolvedThreadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// check if thread is deleted
	if err := router.ValidateThreadNotDeleted(resolvedThreadKey); err != nil {
		router.HandleDeletedError(ctx, err)
		return
	}

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}
	metadata := router.NewRequestMetadata(ctx, author)

	var del models.ThreadDeletePartial
	del.Key = resolvedThreadKey
	del.UpdatedTS = reqtime

	// sync
	if err := router.ValidateAllFieldsNonEmpty(&del); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := queue.GlobalIngestQueue.Enqueue(&types.QueueOp{
		Handler: types.HandlerThreadDelete,
		Payload: &del,
		TS:      reqtime,
		Extras: types.RequestMetadata{
			ApiRole: metadata.ApiRole,
			UserID:  metadata.UserID,
			ReqID:   metadata.ReqID,
			ReqIP:   metadata.ReqIP,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = router.WriteJSON(ctx, map[string]string{"key": resolvedThreadKey})
}

// message operations
func EnqueueCreateMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// sync
	reqtime := timeutil.Now().UnixNano()

	threadKey, ok := router.ExtractParamOrFail(ctx, "threadKey", "thread id missing")
	if !ok {
		return
	}

	// validate
	if err := router.ValidateThreadKey(threadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// validate - del status
	if err := router.ValidateThreadNotDeleted(threadKey); err != nil {
		router.HandleDeletedError(ctx, err)
		return
	}

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}

	// parse
	payload, ok := router.ExtractPayloadOrFail(ctx)
	if !ok {
		return
	}
	var m models.Message
	if err := json.Unmarshal(payload, &m); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid message payload")
		return
	}
	metadata := router.NewRequestMetadata(ctx, author)

	// sync
	messageKey := keys.GenMessagePrvKey(threadKey, fmt.Sprintf("%d", reqtime))
	m.Author = author
	m.Key = messageKey
	m.Thread = threadKey
	m.CreatedTS = reqtime
	m.UpdatedTS = reqtime

	//validate
	if err := router.ValidateAllFieldsNonEmpty(&m); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := queue.GlobalIngestQueue.Enqueue(&types.QueueOp{
		Handler: types.HandlerMessageCreate,
		Payload: &m,
		TS:      reqtime,
		Extras: types.RequestMetadata{
			ApiRole: metadata.ApiRole,
			UserID:  metadata.UserID,
			ReqID:   metadata.ReqID,
			ReqIP:   metadata.ReqIP,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}

	// Track message creation in-flight
	tracking.GlobalInflightTracker.Add(messageKey)

	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = router.WriteJSON(ctx, map[string]string{"key": messageKey})
}

func EnqueueUpdateMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// sync
	reqtime := timeutil.Now().UnixNano()

	// extract
	threadKey, ok := router.ExtractParamOrFail(ctx, "threadKey", "thread id missing")
	if !ok {
		return
	}

	messageKey, ok := router.ExtractParamOrFail(ctx, "id", "message id missing")
	if !ok {
		return
	}

	// resolve provisional keys to final keys
	resolvedMessageKey, err := tracking.GlobalKeyMapper.ResolveKeyOrWait(messageKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, "message not found")
		return
	}

	// validate
	if err := router.ValidateThreadKey(threadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	if err := router.ValidateMessageKey(resolvedMessageKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// validate - del status
	if err := router.ValidateThreadAndMessageNotDeleted(threadKey, resolvedMessageKey); err != nil {
		router.HandleDeletedError(ctx, err)
		return
	}

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}

	// parse
	payload, ok := router.ExtractPayloadOrFail(ctx)
	if !ok {
		return
	}

	var update models.MessageUpdatePartial
	if err := json.Unmarshal(payload, &update); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid message update payload")
		return
	}
	metadata := router.NewRequestMetadata(ctx, author)

	// sync
	update.Key = resolvedMessageKey
	update.Thread = threadKey
	update.UpdatedTS = reqtime

	//validate
	if err := router.ValidateAllFieldsNonEmpty(&update); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := queue.GlobalIngestQueue.Enqueue(&types.QueueOp{
		Handler: types.HandlerMessageUpdate,
		Payload: &update,
		TS:      reqtime,
		Extras: types.RequestMetadata{
			ApiRole: metadata.ApiRole,
			UserID:  metadata.UserID,
			ReqID:   metadata.ReqID,
			ReqIP:   metadata.ReqIP,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = router.WriteJSON(ctx, map[string]string{"key": resolvedMessageKey})
}

func EnqueueDeleteMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	// sync
	reqtime := timeutil.Now().UnixNano()

	// extract
	threadKey, ok := router.ExtractParamOrFail(ctx, "threadKey", "thread id missing")
	if !ok {
		return
	}

	messageKey, ok := router.ExtractParamOrFail(ctx, "id", "message id missing")
	if !ok {
		return
	}

	// resolve provisional keys to final keys
	resolvedMessageKey, err := tracking.GlobalKeyMapper.ResolveKeyOrWait(messageKey)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, "message not found")
		return
	}

	// validate
	if err := router.ValidateThreadKey(threadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	if err := router.ValidateMessageKey(resolvedMessageKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// validate - del status
	if err := router.ValidateThreadAndMessageNotDeleted(threadKey, resolvedMessageKey); err != nil {
		router.HandleDeletedError(ctx, err)
		return
	}

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}

	metadata := router.NewRequestMetadata(ctx, author)

	// sync
	var del models.MessageDeletePartial
	del.Key = resolvedMessageKey
	del.Thread = threadKey
	del.Deleted = true
	del.UpdatedTS = reqtime
	del.Author = author

	//validate
	if err := router.ValidateAllFieldsNonEmpty(&del); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := queue.GlobalIngestQueue.Enqueue(&types.QueueOp{
		Handler: types.HandlerMessageDelete,
		Payload: &del,
		TS:      reqtime,
		Extras: types.RequestMetadata{
			ApiRole: metadata.ApiRole,
			UserID:  metadata.UserID,
			ReqID:   metadata.ReqID,
			ReqIP:   metadata.ReqIP,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = router.WriteJSON(ctx, map[string]string{"key": resolvedMessageKey})
}
