package api

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/api/router"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/timeutil"

	"github.com/valyala/fasthttp"
)

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
	tr := telemetry.Track("api.enqueue_create_thread")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	payload := make([]byte, len(ctx.PostBody()))
	copy(payload, ctx.PostBody())

	// validate
	if err := router.ValidateCreateThreadRequest(ctx); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	tr.Mark("enqueue")

	// enqueue
	reqtime := timeutil.Now().UnixNano()
	metadata := NewRequestMetadata(ctx, author)
	pid := keys.GenThreadPrvKey(fmt.Sprintf("%d", reqtime))

	// parse
	var th models.Thread
	if err := json.Unmarshal(payload, &th); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread payload")
		return
	}
	th.Author = author
	th.CreatedTS = reqtime
	th.UpdatedTS = reqtime

	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerThreadCreate,
		TID:     pid,
		MID:     "",
		Payload: &th,
		TS:      reqtime,
		Extras: queue.RequestMetadata{
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
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, map[string]string{"id": pid})
}
func EnqueueUpdateThread(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.enqueue_update_thread")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}
	payload := make([]byte, len(ctx.PostBody()))
	copy(payload, ctx.PostBody())

	// Validate request
	if err := router.ValidateUpdateThreadRequest(ctx); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	metadata := NewRequestMetadata(ctx, author)
	tr.Mark("enqueue")

	var update models.ThreadUpdatePartial
	if err := json.Unmarshal(payload, &update); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread update payload")
		return
	}

	// Set ID and updated TS
	update.ID = &id
	ts := timeutil.Now().UnixNano()
	update.UpdatedTS = &ts

	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerThreadUpdate,
		TID:     id,
		MID:     "",
		Payload: &update,
		TS:      ts,
		Extras: queue.RequestMetadata{
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
	tr := telemetry.Track("api.enqueue_delete_thread")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}

	// resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}
	metadata := NewRequestMetadata(ctx, author)

	tr.Mark("enqueue")
	var del models.ThreadDeletePartial
	del.ID = &id
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerThreadDelete,
		TID:     id,
		MID:     "",
		Payload: &del,
		TS:      timeutil.Now().UnixNano(),
		Extras: queue.RequestMetadata{
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
	tr := telemetry.Track("api.enqueue_create_message")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	threadID := pathParam(ctx, "threadID")
	if threadID == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}

	// Validate thread ID format
	if err := keys.ValidateThreadKey(threadID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid thread id format")
		return
	}

	payload := make([]byte, len(ctx.PostBody()))
	copy(payload, ctx.PostBody())

	// Validate request
	if err := router.ValidateCreateMessageRequest(ctx); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	reqtime := timeutil.Now().UnixNano()
	metadata := NewRequestMetadata(ctx, author)
	pid := keys.GenMessagePrvKey(threadID, fmt.Sprintf("%d", reqtime))

	// Convert thread ID to full key format for batch processor
	fullThreadKey := keys.GenThreadKey(threadID)

	var m models.Message
	if err := json.Unmarshal(payload, &m); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid message payload")
		return
	}
	m.Author = author
	m.ID = pid
	m.Thread = fullThreadKey
	m.TS = reqtime

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerMessageCreate,
		TID:     fullThreadKey,
		MID:     pid,
		Payload: &m,
		TS:      reqtime,
		Extras: queue.RequestMetadata{
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
	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, map[string]string{"id": pid})
}
func EnqueueUpdateMessage(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.enqueue_update_message")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	threadID := pathParam(ctx, "threadID")
	id := pathParam(ctx, "id")
	if threadID == "" || id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "thread id or message id missing")
		return
	}
	payload := make([]byte, len(ctx.PostBody()))
	copy(payload, ctx.PostBody())

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	ts := timeutil.Now().UnixNano()
	metadata := NewRequestMetadata(ctx, author)

	var update models.MessageUpdatePartial
	if err := json.Unmarshal(payload, &update); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid message update payload")
		return
	}
	update.ID = &id
	update.Thread = &threadID
	if update.TS == nil {
		update.TS = &ts
	}

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerMessageUpdate,
		TID:     threadID,
		MID:     id,
		Payload: &update,
		TS:      ts,
		Extras: queue.RequestMetadata{
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
	tr := telemetry.Track("api.enqueue_delete_message")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "message id missing")
		return
	}

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	metadata := NewRequestMetadata(ctx, author)

	ts := timeutil.Now().UnixNano()
	var del models.DeletePartial
	del.ID = id
	del.Deleted = true
	del.TS = ts
	del.Author = author

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerMessageDelete,
		TID:     "",
		MID:     id,
		Payload: &del,
		TS:      ts,
		Extras: queue.RequestMetadata{
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
