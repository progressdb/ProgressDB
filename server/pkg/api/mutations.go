package api

import (
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/ingest"
	"progressdb/pkg/utils"

	"github.com/valyala/fasthttp"
)

// Mutative HTTP handlers live in this file. They are thin fast-path
// handlers which enqueue raw payloads into the global ingest queue and
// return a 202. Heavy work (validation, auth lookups, KMS, DB writes)
// happens inside the ingest pipeline.

func CreateThread(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	payload := append([]byte(nil), ctx.PostBody()...)
	id := utils.GenThreadID()
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	_ = ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpCreate, Thread: id, ID: "", Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras})
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = json.NewEncoder(ctx).Encode(map[string]string{"id": id})
}

func UpdateThread(ctx *fasthttp.RequestCtx) {
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
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpUpdate, Thread: id, ID: "", Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

func DeleteThread(ctx *fasthttp.RequestCtx) {
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
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpDelete, Thread: id, ID: "", Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

// Message mutators
func CreateThreadMessage(ctx *fasthttp.RequestCtx) {
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
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpCreate, Thread: threadID, ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		if err == ingest.ErrQueueFull {
			utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
			return
		}
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = json.NewEncoder(ctx).Encode(map[string]string{"id": id})
}

// CreateMessage handles POST /v1/messages as a thin enqueue-only wrapper.
func CreateMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	payload := append([]byte(nil), ctx.PostBody()...)
	id := utils.GenID()
	ts := time.Now().UTC().UnixNano()
	extras := map[string]string{
		"role":     string(ctx.Request.Header.Peek("X-Role-Name")),
		"identity": string(ctx.Request.Header.Peek("X-Identity")),
		"reqid":    string(ctx.Request.Header.Peek("X-Request-Id")),
		"remote":   ctx.RemoteAddr().String(),
	}
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpCreate, Thread: "", ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		if err == ingest.ErrQueueFull {
			utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
			return
		}
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
	_ = json.NewEncoder(ctx).Encode(map[string]string{"id": id})
}

func UpdateThreadMessage(ctx *fasthttp.RequestCtx) {
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
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpUpdate, Thread: threadID, ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		if err == ingest.ErrQueueFull {
			utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
			return
		}
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

func DeleteThreadMessage(ctx *fasthttp.RequestCtx) {
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
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpDelete, Thread: "", ID: id, Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		if err == ingest.ErrQueueFull {
			utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
			return
		}
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

// Reactions
func AddReaction(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	threadID := pathParam(ctx, "threadID")
	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "message id missing")
		return
	}
	// Body should contain {"reaction":"👍","identity":"u1"} or similar.
	payload := append([]byte(nil), ctx.PostBody()...)
	ts := time.Now().UTC().UnixNano()
	extras := map[string]string{"role": string(ctx.Request.Header.Peek("X-Role-Name")), "identity": string(ctx.Request.Header.Peek("X-Identity")), "reqid": string(ctx.Request.Header.Peek("X-Request-Id")), "remote": ctx.RemoteAddr().String()}
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpUpdate, Thread: threadID, ID: id, Payload: payload, TS: ts, Extras: extras}); err != nil {
		if err == ingest.ErrQueueFull {
			utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
			return
		}
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

func DeleteReaction(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	identity := pathParam(ctx, "identity")
	id := pathParam(ctx, "id")
	if id == "" || identity == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "message id or identity missing")
		return
	}
	// Minimal payload to indicate deletion: {"remove_reaction_for":"<identity>"}
	payload := []byte(fmt.Sprintf(`{"remove_reaction_for":"%s"}`, identity))
	extras := map[string]string{"role": string(ctx.Request.Header.Peek("X-Role-Name")), "identity": string(ctx.Request.Header.Peek("X-Identity")), "reqid": string(ctx.Request.Header.Peek("X-Request-Id")), "remote": ctx.RemoteAddr().String()}
	if err := ingest.DefaultQueue.TryEnqueue(&ingest.Op{Type: ingest.OpUpdate, Thread: "", ID: id, Payload: payload, TS: time.Now().UTC().UnixNano(), Extras: extras}); err != nil {
		if err == ingest.ErrQueueFull {
			utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "server busy; try again")
			return
		}
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "enqueue failed")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}
