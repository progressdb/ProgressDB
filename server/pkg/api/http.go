package api

import (
	"fmt"
	"progressdb/pkg/router"

	"github.com/valyala/fasthttp"
)

// Handler returns the fasthttp handler for the ProgressDB API.
func Handler() fasthttp.RequestHandler {
	r := router.New()

	// client auth endpoints
	r.POST("/_sign", Sign)
	r.POST("/v1/_sign", Sign)

	// thread metadata operations
	r.POST("/v1/threads", CreateThread)
	r.GET("/v1/threads", ListThreads)
	r.PUT("/v1/threads/{id}", UpdateThread)
	r.GET("/v1/threads/{id}", GetThread)
	r.DELETE("/v1/threads/{id}", DeleteThread)

	// thread message operations
	r.POST("/v1/threads/{threadID}/messages", CreateThreadMessage)
	r.GET("/v1/threads/{threadID}/messages", ListThreadMessages)
	r.GET("/v1/threads/{threadID}/messages/{id}", GetThreadMessage)
	r.PUT("/v1/threads/{threadID}/messages/{id}", UpdateThreadMessage)
	r.DELETE("/v1/threads/{threadID}/messages/{id}", DeleteThreadMessage)

	// thread message reactions
	r.GET("/v1/threads/{threadID}/messages/{id}/versions", ListMessageVersions)
	r.GET("/v1/threads/{threadID}/messages/{id}/reactions", GetReactions)
	r.POST("/v1/threads/{threadID}/messages/{id}/reactions", AddReaction)
	r.DELETE("/v1/threads/{threadID}/messages/{id}/reactions/{identity}", DeleteReaction)

	// helper message endpoints
	r.POST("/v1/messages", CreateMessage)
	r.GET("/v1/messages", ListMessages)

	// admin secure routes
	r.GET("/admin/health", AdminHealth)
	r.GET("/admin/stats", AdminStats)
	r.GET("/admin/threads", AdminListThreads)
	r.GET("/admin/keys", AdminListKeys)
	r.GET("/admin/keys/{key}", AdminGetKey)
	r.POST("/admin/encryption/rotate-thread-dek", AdminEncryptionRotateThreadDEK)
	r.POST("/admin/test-retention-run", AdminTestRetentionRun)
	return r.Handler
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
