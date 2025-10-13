package api

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"progressdb/pkg/auth"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"

	"github.com/valyala/fasthttp"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	titleQ := strings.TrimSpace(string(ctx.QueryArgs().Peek("title")))
	slugQ := strings.TrimSpace(string(ctx.QueryArgs().Peek("slug")))

	vals, err := store.ListThreads()
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

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

	_ = json.NewEncoder(ctx).Encode(struct {
		Threads []models.Thread `json:"threads"`
	}{Threads: out})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
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

	stored, err := store.GetThread(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

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

	_, _ = ctx.WriteString(stored)
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	threadID := pathParam(ctx, "threadID")

	if stored, err := store.GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(stored), &th); err == nil {
			if th.Deleted {
				utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "thread not found")
				return
			}
		}
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

	_ = json.NewEncoder(ctx).Encode(struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}{Thread: threadID, Messages: out})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	id := pathParam(ctx, "id")
	stored, err := store.GetLatestMessage(id)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, err.Error())
		return
	}

	var message models.Message
	if err := json.Unmarshal([]byte(stored), &message); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "failed to parse message")
		return
	}

	if message.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match")
		return
	}

	_ = json.NewEncoder(ctx).Encode(message)
}

func ReadMessageReactions(ctx *fasthttp.RequestCtx) {
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
