package api

import (
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/utils"

	"github.com/valyala/fasthttp"
)

func handleQueueError(ctx *fasthttp.RequestCtx, err error) {
	if err == nil {
		return
	}
	switch err {
	case queue.ErrQueueFull:
		utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
	case queue.ErrQueueClosed:
		utils.JSONErrorFast(ctx, fasthttp.StatusServiceUnavailable, "server shutting down")
	default:
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
	}
}
// Mutating endpoints enqueue requests and return 202; processing happens in the ingest pipeline.

// thread management
func EnqueueCreateThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	payload := append([]byte(nil), ctx.PostBody()...)
	id := utils.GenThreadID()
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerThreadCreate, Thread: id, ID: "", Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = json.NewEncoder(ctx).Encode(map[string]string{"id": id})
}
func EnqueueUpdateThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}
	payload := append([]byte(nil), ctx.PostBody()...)
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerThreadUpdate, Thread: id, ID: "", Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}
func EnqueueDeleteThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}
	payload := []byte(fmt.Sprintf(`{"id":"%s"}`, id))
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerThreadDelete, Thread: id, ID: "", Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

// message operations
func EnqueueCreateMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	threadID := pathParam(ctx, "threadID")
	if threadID == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}
	payload := append([]byte(nil), ctx.PostBody()...)
	id := utils.GenID()
	ts := time.Now().UTC().UnixNano()
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerMessageCreate, Thread: threadID, ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = json.NewEncoder(ctx).Encode(map[string]string{"id": id})
}
func EnqueueUpdateMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	threadID := pathParam(ctx, "threadID")
	id := pathParam(ctx, "id")
	if threadID == "" || id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id or message id missing")
		return
	}
	payload := append([]byte(nil), ctx.PostBody()...)
	ts := time.Now().UTC().UnixNano()
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerMessageUpdate, Thread: threadID, ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}
func EnqueueDeleteMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "message id missing")
		return
	}
	payload := []byte(fmt.Sprintf(`{"id":"%s"}`, id))
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerMessageDelete, Thread: "", ID: id, Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

// message reactions
func EnqueueAddReaction(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "message id missing")
		return
	}
	// Body should contain {"reaction":"👍","identity":"u1"} or similar.
	payload := append([]byte(nil), ctx.PostBody()...)
	ts := time.Now().UTC().UnixNano()
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerReactionAdd, Thread: "", ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}
func EnqueueDeleteReaction(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	identity := pathParam(ctx, "identity")
	id := pathParam(ctx, "id")
	if id == "" || identity == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "message id or identity missing")
		return
	}
	// Minimal payload to indicate deletion: {"remove_reaction_for":"<identity>"}
	payload := []byte(fmt.Sprintf(`{"remove_reaction_for":"%s"}`, identity))
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := queue.DefaultQueue.TryEnqueue(&queue.Op{Handler: queue.HandlerReactionDelete, Thread: "", ID: id, Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		handleQueueError(ctx, err)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}
