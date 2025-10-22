package api

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/telemetry"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/store/db/index"
	message_store "progressdb/pkg/store/messages"
	thread_store "progressdb/pkg/store/threads"
)

func ReadThreadsList(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_threads_list")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	titleQ := strings.TrimSpace(string(ctx.QueryArgs().Peek("title")))
	slugQ := strings.TrimSpace(string(ctx.QueryArgs().Peek("slug")))

	tr.Mark("get_user_threads")
	var threadIDs []string
	var err error

	if author != "" {
		threadIDs, err = index.GetUserThreads(author)
		if err != nil {
			router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
	} else {
		vals, err := thread_store.ListThreads()
		if err != nil {
			router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
			return
		}
		threadIDs = make([]string, 0, len(vals))
		for _, raw := range vals {
			var th models.Thread
			if err := json.Unmarshal([]byte(raw), &th); err != nil {
				continue
			}
			threadIDs = append(threadIDs, th.ID)
		}
	}

	tr.Mark("fetch_threads")
	out := make([]models.Thread, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		thread, validationErr := ValidateThread(threadID, author, false)
		if validationErr != nil {
			continue
		}
		if titleQ != "" && !strings.Contains(strings.ToLower(thread.Title), strings.ToLower(titleQ)) {
			continue
		}
		if slugQ != "" && thread.Slug != slugQ {
			continue
		}
		out = append(out, *thread)
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, struct {
		Threads []models.Thread `json:"threads"`
	}{Threads: out})
}

func ReadThreadItem(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_thread_item")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}

	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	tr.Mark("validate_thread")
	thread, validationErr := ValidateThread(id, author, true)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("write_response")
	threadJSON, _ := json.Marshal(thread)
	_, _ = ctx.Write(threadJSON)
}

func ReadThreadMessages(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_thread_messages")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	threadID := pathParam(ctx, "threadID")

	tr.Mark("validate_thread")
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
	msgs, err := message_store.ListMessages(threadID)
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	limit := -1
	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit >= 0 {
			limit = parsedLimit
		}
	}

	// used by backends - to include deleted fields
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

	if limit > 0 && limit < len(out) {
		out = out[len(out)-limit:]
	}

	if len(out) == 0 {
		tr.Mark("encode_empty_response")
		_ = router.WriteJSON(ctx, struct {
			Thread   string           `json:"thread"`
			Messages []models.Message `json:"messages"`
			Metadata interface{}      `json:"metadata,omitempty"`
		}{Thread: threadID, Messages: out})
		return
	}

	if author != "" && !authorFound {
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "author not found in any message in this thread")
		return
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
		Metadata interface{}      `json:"metadata,omitempty"`
	}{Thread: threadID, Messages: out, Metadata: threadIndexes})
}

func ReadThreadMessage(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_thread_message")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	id := pathParam(ctx, "id")
	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "message id missing")
		return
	}

	tr.Mark("validate_message")
	message, validationErr := ValidateMessage(id, author, true)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	tr.Mark("encode_response")
	_ = router.WriteJSON(ctx, message)
}

func ReadMessageReactions(ctx *fasthttp.RequestCtx) {
	tr := telemetry.Track("api.read_message_reactions")
	defer tr.Finish()

	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	threadID := pathParam(ctx, "threadID")

	if id == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "missing id")
		return
	}

	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		return
	}

	tr.Mark("validate_message")
	message, validationErr := ValidateMessage(id, author, false)
	if validationErr != nil {
		WriteValidationError(ctx, validationErr)
		return
	}

	if threadID != "" && message.Thread != threadID {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, "message not found in thread")
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
	_ = router.WriteJSON(ctx, struct {
		ID        string      `json:"id"`
		Reactions interface{} `json:"reactions"`
	}{ID: id, Reactions: out})
}
