package api

import (
	"fmt"
	"net/http"
	"net/http/pprof"

	"progressdb/pkg/api/router"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

// wrapHTTPHandler wraps an http.Handler to work with fasthttp.
func wrapHTTPHandler(h http.Handler) func(ctx *fasthttp.RequestCtx) {
	return func(ctx *fasthttp.RequestCtx) {
		fasthttpadaptor.NewFastHTTPHandler(h)(ctx)
	}
}

// RegisterRoutes wires all API routes onto the provided router.
func RegisterRoutes(r *router.Router) {
	// client auth endpoints
	r.POST("/_sign", Sign)
	r.POST("/v1/_sign", Sign)

	// thread metadata operations
	r.POST("/v1/threads", EnqueueCreateThread)
	r.GET("/v1/threads", ReadThreadsList)
	r.PUT("/v1/threads/{id}", EnqueueUpdateThread)
	r.GET("/v1/threads/{id}", ReadThreadItem)
	r.DELETE("/v1/threads/{id}", EnqueueDeleteThread)

	// thread message operations
	r.POST("/v1/threads/{threadID}/messages", EnqueueCreateMessage)
	r.GET("/v1/threads/{threadID}/messages", ReadThreadMessages)
	r.GET("/v1/threads/{threadID}/messages/{id}", ReadThreadMessage)
	r.PUT("/v1/threads/{threadID}/messages/{id}", EnqueueUpdateMessage)
	r.DELETE("/v1/threads/{threadID}/messages/{id}", EnqueueDeleteMessage)

	// thread message reactions
	// r.GET("/v1/threads/{threadID}/messages/{id}/versions", ListMessageVersions)
	r.GET("/v1/threads/{threadID}/messages/{id}/reactions", ReadMessageReactions)
	r.POST("/v1/threads/{threadID}/messages/{id}/reactions", EnqueueAddReaction)
	r.DELETE("/v1/threads/{threadID}/messages/{id}/reactions/{identity}", EnqueueDeleteReaction)

	// // helper message endpoints
	// r.POST("/v1/messages", CreateMessage)
	// r.GET("/v1/messages", ListMessages)

	// admin data routes
	r.GET("/admin/health", AdminHealth)
	r.GET("/admin/stats", AdminStats)
	r.GET("/admin/threads", AdminListThreads)
	r.GET("/admin/keys", AdminListKeys)
	r.GET("/admin/keys/{key}", AdminGetKey)

	// admin enc routes
	r.POST("/admin/encryption/rotate-thread-dek", AdminEncryptionRotateThreadDEK)
	r.POST("/admin/encryption/rewrap-deks", AdminEncryptionRewrapDEKs)
	r.POST("/admin/encryption/encrypt-existing", AdminEncryptionEncryptExisting)
	r.POST("/admin/encryption/generate-kek", AdminEncryptionGenerateKEK)

	// admin pprof routes
	r.GET("/admin/debug/pprof/", wrapHTTPHandler(http.HandlerFunc(pprof.Index)))
	r.GET("/admin/debug/pprof/cmdline", wrapHTTPHandler(http.HandlerFunc(pprof.Cmdline)))
	r.GET("/admin/debug/pprof/profile", wrapHTTPHandler(http.HandlerFunc(pprof.Profile)))
	r.GET("/admin/debug/pprof/symbol", wrapHTTPHandler(http.HandlerFunc(pprof.Symbol)))
	r.GET("/admin/debug/pprof/trace", wrapHTTPHandler(http.HandlerFunc(pprof.Trace)))
}

// Handler returns the fasthttp handler for the ProgressDB API.
func Handler() fasthttp.RequestHandler {
	r := router.New()
	RegisterRoutes(r)
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
