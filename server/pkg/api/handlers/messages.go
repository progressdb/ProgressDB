package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"progressdb/pkg/auth"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"
	"progressdb/pkg/validation"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
)

// RegisterMessagesFast registers message routes on the fasthttp router.
func RegisterMessagesFast(r *router.Router) {
	r.POST("/v1/messages", createMessageFast)
	r.GET("/v1/messages", listMessagesFast)
}

func createMessageFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	telemetry.SetRequestOpNoCtx("create_message")
	defer telemetry.StartSpanNoCtx("create_message.handler")()

	body := ctx.PostBody()
	if len(body) == 0 {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}

	var msg models.Message
	decodeSpan := telemetry.StartSpanNoCtx("decode_body")
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&msg); err != nil {
		decodeSpan()
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}
	decodeSpan()

	authSpan := telemetry.StartSpanNoCtx("resolve_author")
	if author, code, message := auth.ResolveAuthorFromRequestFast(ctx, msg.Author); code != 0 {
		authSpan()
		utils.JSONErrorFast(ctx, code, message)
		return
	} else {
		msg.Author = author
	}
	authSpan()

	if msg.Role == "" {
		msg.Role = "user"
	}

	// Create or validate thread association.
	if msg.Thread == "" {
		createThreadSpan := telemetry.StartSpanNoCtx("create_thread")
		thread, err := createThreadInternal(context.Background(), msg.Author, defaultThreadTitle())
		createThreadSpan()
		if err != nil {
			utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "failed to create thread")
			return
		}
		msg.Thread = thread.ID
	} else {
		getThreadSpan := telemetry.StartSpanNoCtx("get_thread")
		stored, err := store.GetThread(msg.Thread)
		getThreadSpan()
		if err != nil {
			utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "thread not found")
			return
		}
		var thread models.Thread
		if err := json.Unmarshal([]byte(stored), &thread); err != nil {
			utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "failed to parse thread")
			return
		}
		role := string(ctx.Request.Header.Peek("X-Role-Name"))
		if role != "admin" && thread.Author != msg.Author {
			utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match thread")
			return
		}
		if thread.Deleted && role != "admin" {
			utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "thread deleted")
			return
		}
	}

	msg.ID = utils.GenID()
	if msg.TS == 0 {
		msg.TS = time.Now().UTC().UnixNano()
	}

	validateSpan := telemetry.StartSpanNoCtx("validate_message")
	if err := validation.ValidateMessage(msg); err != nil {
		validateSpan()
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}
	validateSpan()

	saveSpan := telemetry.StartSpanNoCtx("save_message")
	if err := store.SaveMessage(context.Background(), msg.Thread, msg.ID, msg); err != nil {
		saveSpan()
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	saveSpan()

	logger.Info("message_created", "thread", msg.Thread, "id", msg.ID)
	encodeSpan := telemetry.StartSpanNoCtx("encode_response")
	_ = json.NewEncoder(ctx).Encode(msg)
	encodeSpan()
}

func listMessagesFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	threadID := string(ctx.QueryArgs().Peek("thread"))
	if threadID == "" {
		threadID = "default"
	}

	msgs, err := store.ListMessages(threadID)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if limit, err := strconv.Atoi(limStr); err == nil && limit >= 0 && limit < len(msgs) {
			msgs = msgs[len(msgs)-limit:]
		}
	}

	includeDeleted := string(ctx.QueryArgs().Peek("include_deleted")) == "true"

	latest := make(map[string]models.Message)
	for _, encoded := range msgs {
		var m models.Message
		if err := json.Unmarshal([]byte(encoded), &m); err != nil {
			continue
		}
		current, ok := latest[m.ID]
		if !ok || m.TS >= current.TS {
			latest[m.ID] = m
		}
	}

	out := make([]models.Message, 0, len(latest))
	for _, v := range latest {
		if v.Deleted && !includeDeleted {
			continue
		}
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].TS < out[j].TS })

	logger.Info("messages_list", "thread", threadID, "count", len(out))
	_ = json.NewEncoder(ctx).Encode(struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}{Thread: threadID, Messages: out})
}

func listMessageVersionsFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	threadID := pathParam(ctx, "threadID")

	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "missing id")
		return
	}

	if threadID != "" {
		stored, err := store.GetLatestMessage(id)
		if err != nil {
			utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
			return
		}
		var msg models.Message
		if err := json.Unmarshal([]byte(stored), &msg); err != nil {
			utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "failed to parse message")
			return
		}
		if msg.Thread != threadID {
			utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "message not found in thread")
			return
		}
	}

	versions, err := store.ListMessageVersions(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	out := make([]json.RawMessage, 0, len(versions))
	for _, v := range versions {
		out = append(out, json.RawMessage(v))
	}

	_ = json.NewEncoder(ctx).Encode(struct {
		ID       string            `json:"id"`
		Versions []json.RawMessage `json:"versions"`
	}{ID: id, Versions: out})
}

func getReactionsFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	threadID := pathParam(ctx, "threadID")

	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "missing id")
		return
	}

	stored, err := store.GetLatestMessage(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	var msg models.Message
	if err := json.Unmarshal([]byte(stored), &msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "invalid stored message")
		return
	}

	if threadID != "" && msg.Thread != threadID {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "message not found in thread")
		return
	}

	type reaction struct {
		ID       string `json:"id"`
		Reaction string `json:"reaction"`
	}

	out := make([]reaction, 0, len(msg.Reactions))
	for k, v := range msg.Reactions {
		out = append(out, reaction{ID: k, Reaction: v})
	}

	_ = json.NewEncoder(ctx).Encode(struct {
		ID        string      `json:"id"`
		Reactions interface{} `json:"reactions"`
	}{ID: id, Reactions: out})
}

func addReactionFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	threadID := pathParam(ctx, "threadID")

	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "missing id")
		return
	}

	var payload struct {
		ID       string `json:"id"`
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&payload); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}

	identity := payload.ID
	if identity == "" {
		identity = string(ctx.Request.Header.Peek("X-Identity"))
	}
	if identity == "" || payload.Reaction == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "missing id or reaction")
		return
	}

	stored, err := store.GetLatestMessage(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	var msg models.Message
	if err := json.Unmarshal([]byte(stored), &msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "invalid stored message")
		return
	}
	if threadID != "" && msg.Thread != threadID {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "message not found in thread")
		return
	}

	if msg.Reactions == nil {
		msg.Reactions = make(map[string]string)
	}
	msg.Reactions[identity] = payload.Reaction
	msg.TS = time.Now().UTC().UnixNano()

	if err := store.SaveMessage(context.Background(), msg.Thread, msg.ID, msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(ctx).Encode(msg)
}

func deleteReactionFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	identity := pathParam(ctx, "identity")
	threadID := pathParam(ctx, "threadID")

	if id == "" || identity == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "missing id or identity")
		return
	}

	stored, err := store.GetLatestMessage(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}
	var msg models.Message
	if err := json.Unmarshal([]byte(stored), &msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "invalid stored message")
		return
	}
	if threadID != "" && msg.Thread != threadID {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "message not found in thread")
		return
	}

	if msg.Reactions != nil {
		delete(msg.Reactions, identity)
	}
	msg.TS = time.Now().UTC().UnixNano()

	if err := store.SaveMessage(context.Background(), msg.Thread, msg.ID, msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

func pathParam(ctx *fasthttp.RequestCtx, key string) string {
	if v := ctx.UserValue(key); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}
