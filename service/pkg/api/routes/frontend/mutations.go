package frontend

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
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
			Role:   metadata.Role,
			UserID: metadata.UserID,
			ReqID:  metadata.ReqID,
			Remote: metadata.Remote,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
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

	// validate
	if err := router.ValidateThreadKey(threadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
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
	update.Key = threadKey
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
			Role:   metadata.Role,
			UserID: metadata.UserID,
			ReqID:  metadata.ReqID,
			Remote: metadata.Remote,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

func EnqueueDeleteThread(ctx *fasthttp.RequestCtx) {
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

	// resolve
	author, authErr := router.ValidateAuthor(ctx, "")
	if authErr != nil {
		router.WriteValidationError(ctx, authErr)
		return
	}
	metadata := router.NewRequestMetadata(ctx, author)

	var del models.ThreadDeletePartial
	del.Key = threadKey
	del.TS = reqtime

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
			Role:   metadata.Role,
			UserID: metadata.UserID,
			ReqID:  metadata.ReqID,
			Remote: metadata.Remote,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
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
	m.TS = reqtime

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
			Role:   metadata.Role,
			UserID: metadata.UserID,
			ReqID:  metadata.ReqID,
			Remote: metadata.Remote,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
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

	// validate
	if err := router.ValidateThreadKey(threadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	if err := router.ValidateMessageKey(messageKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
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
	update.Key = messageKey
	update.Thread = threadKey
	update.TS = reqtime

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
			Role:   metadata.Role,
			UserID: metadata.UserID,
			ReqID:  metadata.ReqID,
			Remote: metadata.Remote,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
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

	// validate
	if err := router.ValidateThreadKey(threadKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	if err := router.ValidateMessageKey(messageKey); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
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
	del.Key = messageKey
	del.Thread = threadKey
	del.Deleted = true
	del.TS = reqtime
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
			Role:   metadata.Role,
			UserID: metadata.UserID,
			ReqID:  metadata.ReqID,
			Remote: metadata.Remote,
		},
	}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}
