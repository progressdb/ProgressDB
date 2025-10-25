package api

import (
	"fmt"

	"progressdb/pkg/api/router"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
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
	payload := append([]byte(nil), ctx.PostBody()...)

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

	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerThreadCreate,
		TID:     pid,
		MID:     "",
		Payload: payload,
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
	payload := append([]byte(nil), ctx.PostBody()...)

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
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerThreadUpdate,
		TID:     id,
		MID:     "",
		Payload: payload,
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
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerThreadDelete,
		TID:     id,
		MID:     "",
		Payload: nil,
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

	payload := append([]byte(nil), ctx.PostBody()...)

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

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerMessageCreate,
		TID:     fullThreadKey,
		MID:     pid,
		Payload: payload,
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
	payload := append([]byte(nil), ctx.PostBody()...)

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	ts := timeutil.Now().UnixNano()
	metadata := NewRequestMetadata(ctx, author)

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerMessageUpdate,
		TID:     threadID,
		MID:     id,
		Payload: payload,
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
	payload := []byte(fmt.Sprintf(`{"id":"%s"}`, id))

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	metadata := NewRequestMetadata(ctx, author)

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerMessageDelete,
		TID:     "",
		MID:     id,
		Payload: payload,
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

// message reactions
func EnqueueAddReaction(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.enqueue_add_reaction")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "message id missing")
		return
	}
	// Body should contain {"reaction":"👍","identity":"u1"} or similar.
	payload := append([]byte(nil), ctx.PostBody()...)

	// Validate request
	if err := router.ValidateReactionRequest(ctx); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	ts := timeutil.Now().UnixNano()
	metadata := NewRequestMetadata(ctx, author)

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerReactionAdd,
		TID:     "",
		MID:     id,
		Payload: payload,
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
func EnqueueDeleteReaction(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.enqueue_delete_reaction")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")
	identity := pathParam(ctx, "identity")
	id := pathParam(ctx, "id")
	if id == "" || identity == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "message id or identity missing")
		return
	}
	// Minimal payload to indicate deletion: {"remove_reaction_for":"<identity>"}
	payload := []byte(fmt.Sprintf(`{"remove_reaction_for":"%s"}`, identity))

	// Resolve author
	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	metadata := NewRequestMetadata(ctx, author)

	tr.Mark("enqueue")
	if err := queue.GlobalIngestQueue.Enqueue(&queue.QueueOp{
		Handler: queue.HandlerReactionDelete,
		TID:     "",
		MID:     id,
		Payload: payload,
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
