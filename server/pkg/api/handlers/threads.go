package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"progressdb/pkg/auth"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"
	"progressdb/pkg/validation"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
)

// RegisterThreadsFast registers all thread-related routes on the fasthttp router.
func RegisterThreadsFast(r *router.Router) {
	r.POST("/v1/threads", createThreadFast)
	r.GET("/v1/threads", listThreadsFast)
	r.PUT("/v1/threads/{id}", updateThreadFast)
	r.GET("/v1/threads/{id}", getThreadFast)
	r.DELETE("/v1/threads/{id}", deleteThreadFast)

	r.POST("/v1/threads/{threadID}/messages", createThreadMessageFast)
	r.GET("/v1/threads/{threadID}/messages", listThreadMessagesFast)
	r.GET("/v1/threads/{threadID}/messages/{id}", getThreadMessageFast)
	r.PUT("/v1/threads/{threadID}/messages/{id}", updateThreadMessageFast)
	r.DELETE("/v1/threads/{threadID}/messages/{id}", deleteThreadMessageFast)

	r.GET("/v1/threads/{threadID}/messages/{id}/versions", listMessageVersionsFast)
	r.GET("/v1/threads/{threadID}/messages/{id}/reactions", getReactionsFast)
	r.POST("/v1/threads/{threadID}/messages/{id}/reactions", addReactionFast)
	r.DELETE("/v1/threads/{threadID}/messages/{id}/reactions/{identity}", deleteReactionFast)
}

func createThreadFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	var thread models.Thread
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&thread); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}

	if author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, thread.Author); code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	} else {
		thread.Author = author
	}

	created, err := createThreadInternal(context.Background(), thread.Author, thread.Title)
	if err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(ctx).Encode(created)
}

func createThreadInternal(ctx context.Context, author, title string) (models.Thread, error) {
	var t models.Thread
	if title == "" {
		title = defaultThreadTitle()
	}

	start := time.Now()
	logger.Debug("thread_create_checkpoint", "step", "start", "author", author)

	prepSpan := telemetry.StartSpanNoCtx("create_thread.prepare")
	t.ID = utils.GenThreadID()
	t.Author = author
	t.Title = title
	t.Slug = utils.MakeSlug(t.Title, t.ID)
	t.CreatedTS = time.Now().UTC().UnixNano()
	t.UpdatedTS = t.CreatedTS
	prepSpan()
	logger.Debug("thread_create_checkpoint", "step", "prepared", "elapsed_ms", time.Since(start).Milliseconds(), "thread", t.ID)

	t.LastSeq = 0

	encCheckSpan := telemetry.StartSpanNoCtx("create_thread.enc_check")
	encStart := time.Now()
	if security.EncryptionEnabled() {
		if kms.IsProviderEnabled() {
			logger.Info("provisioning_thread_kms", "thread", t.ID)
			kmsSpan := telemetry.StartSpanNoCtx("kms.create_dek_for_thread")
			kmsStart := time.Now()
			keyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(t.ID)
			kmsSpan()
			logger.Debug("thread_create_checkpoint", "step", "kms_done", "elapsed_ms", time.Since(kmsStart).Milliseconds(), "thread", t.ID)
			if err != nil {
				return models.Thread{}, err
			}
			t.KMS = &models.KMSMeta{KeyID: keyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
		} else {
			logger.Info("encryption_enabled_but_no_kms_provider", "thread", t.ID)
		}
	}
	encCheckSpan()
	logger.Debug("thread_create_checkpoint", "step", "enc_check_done", "elapsed_ms", time.Since(encStart).Milliseconds(), "thread", t.ID)

	marshalSpan := telemetry.StartSpanNoCtx("create_thread.marshal")
	marStart := time.Now()
	payload, _ := json.Marshal(t)
	marshalSpan()
	logger.Debug("thread_create_checkpoint", "step", "marshal_done", "elapsed_ms", time.Since(marStart).Milliseconds(), "thread", t.ID)

	saveSpan := telemetry.StartSpanNoCtx("store.save_thread")
	saveStart := time.Now()
	if err := store.SaveThread(t.ID, string(payload)); err != nil {
		saveSpan()
		return models.Thread{}, err
	}
	saveSpan()
	logger.Debug("thread_create_checkpoint", "step", "saved", "elapsed_ms", time.Since(saveStart).Milliseconds(), "thread", t.ID)
	logger.Debug("thread_create_checkpoint", "step", "done", "total_elapsed_ms", time.Since(start).Milliseconds(), "thread", t.ID)
	return t, nil
}

func listThreadsFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		if role == "admin" {
			author = ""
		} else {
			utils.JSONErrorFast(ctx, code, msg)
			return
		}
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
		if th.Deleted && role != "admin" {
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

func getThreadFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		if role == "admin" {
			author = ""
		} else {
			utils.JSONErrorFast(ctx, code, msg)
			return
		}
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
	if th.Deleted && role != "admin" {
		utils.JSONErrorFast(ctx, fasthttp.StatusNotFound, "thread not found")
		return
	}
	if author != "" && th.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match")
		return
	}

	_, _ = ctx.WriteString(stored)
}

func deleteThreadFast(ctx *fasthttp.RequestCtx) {
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

	if err := store.SoftDeleteThread(id, author); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

func updateThreadFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	id := pathParam(ctx, "id")
	if id == "" {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "thread id missing")
		return
	}

	var payload models.Thread
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&payload); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, payload.Author)
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

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	if role != "admin" && th.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match")
		return
	}

	if payload.Title != "" {
		th.Title = payload.Title
		th.Slug = utils.MakeSlug(th.Title, th.ID)
	}
	th.UpdatedTS = time.Now().UTC().UnixNano()

	buf, _ := json.Marshal(th)
	if err := store.SaveThread(th.ID, string(buf)); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	_ = json.NewEncoder(ctx).Encode(th)
}

func createThreadMessageFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	threadID := pathParam(ctx, "threadID")

	var msg models.Message
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}

	if author, code, message := auth.ResolveAuthorFromRequestFast(ctx, msg.Author); code != 0 {
		utils.JSONErrorFast(ctx, code, message)
		return
	} else {
		msg.Author = author
	}

	if msg.Role == "" {
		msg.Role = "user"
	}
	msg.Thread = threadID
	msg.ID = utils.GenID()
	if msg.TS == 0 {
		msg.TS = time.Now().UTC().UnixNano()
	}

	if stored, err := store.GetThread(msg.Thread); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(stored), &th); err == nil {
			if th.Deleted && string(ctx.Request.Header.Peek("X-Role-Name")) != "admin" {
				utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "thread deleted")
				return
			}
		}
	}

	if err := validation.ValidateMessage(msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := store.SaveMessage(context.Background(), msg.Thread, msg.ID, msg); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	_ = json.NewEncoder(ctx).Encode(msg)
}

func listThreadMessagesFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		if role == "admin" {
			author = ""
		} else {
			utils.JSONErrorFast(ctx, code, msg)
			return
		}
	}

	threadID := pathParam(ctx, "threadID")

	if stored, err := store.GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(stored), &th); err == nil {
			if th.Deleted && string(ctx.Request.Header.Peek("X-Role-Name")) != "admin" {
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

	if role != "admin" && author != "" && !authorFound {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author not found in any message in this thread")
		return
	}

	_ = json.NewEncoder(ctx).Encode(struct {
		Thread   string           `json:"thread"`
		Messages []models.Message `json:"messages"`
	}{Thread: threadID, Messages: out})
}

func getThreadMessageFast(ctx *fasthttp.RequestCtx) {
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

func updateThreadMessageFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	threadID := pathParam(ctx, "threadID")
	id := pathParam(ctx, "id")

	author, code, msg := auth.ResolveAuthorFromRequestFast(ctx, "")
	if code != 0 {
		utils.JSONErrorFast(ctx, code, msg)
		return
	}

	var message models.Message
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&message); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid json")
		return
	}

	if message.Author != "" && message.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author in body does not match verified author")
		return
	}

	message.Author = author
	message.ID = id
	message.Thread = threadID
	if message.TS == 0 {
		message.TS = time.Now().UTC().UnixNano()
	}

	if err := validation.ValidateMessage(message); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	if err := store.SaveMessage(context.Background(), message.Thread, message.ID, message); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	_ = json.NewEncoder(ctx).Encode(message)
}

func deleteThreadMessageFast(ctx *fasthttp.RequestCtx) {
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
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "invalid stored message")
		return
	}

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	if role != "admin" && message.Author != author {
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "author does not match")
		return
	}

	message.Deleted = true
	message.TS = time.Now().UTC().UnixNano()

	if err := store.SaveMessage(context.Background(), message.Thread, message.ID, message); err != nil {
		utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

func defaultThreadTitle() string {
	vals, err := store.ListThreads()
	if err != nil {
		return "New Thread"
	}
	return fmt.Sprintf("New Thread #%d", len(vals)+1)
}
