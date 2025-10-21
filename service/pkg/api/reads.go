package api

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"progressdb/pkg/auth"
	"progressdb/pkg/models"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"

	"github.com/valyala/fasthttp"

	thread_store "progressdb/pkg/store/threads"
	message_store "progressdb/pkg/store/messages"

)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_threads_list")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	titleQ := strings.TrimSpace(string(ctx.QueryArgs().Peek("title")))
	slugQ := strings.TrimSpace(string(ctx.QueryArgs().Peek("slug")))

	tr.Mark("list_threads")
	vals, err := thread_store.ListThreads()
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	tr.Mark("filter_threads")
	out := make([]models.Thread, 0, len(vals))
	for _, raw := range vals {
		var th models.Thread
		if err := json.Unmarshal([]byte(raw), &th); err != nil {
			continue
		}
		if th.Deleted {
			continue
		}
		if author != "" && th.Author != author {
			continue
		}
		if titleQ != "" && !strings.Contains(strings.ToLower(th.Title), strings.ToLower(titleQ)) {
			continue
		}
		if slugQ != "" && th.Slug != slugQ {
			continue
		}
		out = append(out, th)
	}

	tr.Mark("encode_response")
	_ = json.NewEncoder(ctx).Encode(struct {
		Threads []models.Thread `json:"threads"`
	}{Threads: out})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_thread_item")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	tr.Mark("get_thread")
	stored, err := thread_store.GetThread(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

	tr.Mark("parse_thread")
	var th models.Thread
	if err := json.Unmarshal([]byte(stored), &th); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "failed to parse thread")
		return
	}
	if th.Deleted {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "thread not found")
		return
	}
	if author != "" && th.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match")
		return
	}

	tr.Mark("write_response")
	_, _ = ctx.WriteString(stored)
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_thread_messages")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	threadID := pathParam(ctx, "threadID")

	tr.Mark("check_thread")
	if stored, err := thread_store.GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(stored), &th); err == nil {
			if th.Deleted {
				utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "thread not found")
				return
			}
		}
	}

	tr.Mark("list_messages")
	msgs, err := message_store.ListMessages(threadID)
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

	tr.Mark("process_messages")
	latest := make(map[string]models.Message)
	authorFound := false
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
		if v.Author == author {
			authorFound = true
		}
		if v.Deleted && !includeDeleted {
			continue
		}
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].TS < out[j].TS })

	if len(out) == 0 {
		tr.Mark("encode_empty_response")
		_ = json.NewEncoder(ctx).Encode(struct {
			Thread   string           `json:"thread"`
			Messages []models.Message `json:"messages"`
		}{Thread: threadID, Messages: out})
		return
	}

	if author != "" && !authorFound {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author not found in any message in this thread")
		return
	}

	tr.Mark("encode_response")
	_ = json.NewEncoder(ctx).Encode(struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}{Thread: threadID, Messages: out})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_thread_message")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	id := pathParam(ctx, "id")
	tr.Mark("get_message")
	stored, err := message_store.GetLatestMessage(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

	tr.Mark("parse_message")
	var message models.Message
	if err := json.Unmarshal([]byte(stored), &message); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "failed to parse message")
		return
	}

	if message.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match")
		return
	}

	tr.Mark("encode_response")
	_ = json.NewEncoder(ctx).Encode(message)
}

func ReadMessageReactions(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_message_reactions")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	threadID := pathParam(ctx, "threadID")

	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "missing id")
		return
	}

	tr.Mark("get_message")
	stored, err := message_store.GetLatestMessage(id)
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

	tr.Mark("process_reactions")
	type reaction struct {
		ID       string `json:"id"`
		Reaction string `json:"reaction"`
	}

	out := make([]reaction, 0, len(msg.Reactions))
	for k, v := range msg.Reactions {
		out = append(out, reaction{ID: k, Reaction: v})
	}

	tr.Mark("encode_response")
	_ = json.NewEncoder(ctx).Encode(struct {
		ID        string      `json:"id"`
		Reactions interface{} `json:"reactions"`
	}{ID: id, Reactions: out})
}
