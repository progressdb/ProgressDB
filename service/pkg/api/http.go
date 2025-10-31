package api

import (
	"net/http"
	"net/http/pprof"
	"runtime"

	"progressdb/pkg/api/router"
	adminRoutes "progressdb/pkg/api/routes/admin"
	backendRoutes "progressdb/pkg/api/routes/backend"
	frontendRoutes "progressdb/pkg/api/routes/frontend"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var (
	goroutines = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "go_goroutines",
			Help: "Number of active goroutines.",
		},
		func() float64 { return float64(runtime.NumGoroutine()) },
	)

	gcPauseTotal = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "go_gc_pause_total_ns",
			Help: "Total GC pause time in nanoseconds.",
		},
		func() float64 {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			return float64(stats.PauseTotalNs)
		},
	)

	heapAlloc = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "go_heap_alloc_bytes",
			Help: "Current heap allocation in bytes.",
		},
		func() float64 {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			return float64(stats.HeapAlloc)
		},
	)

	heapSys = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "go_heap_sys_bytes",
			Help: "Total heap size in bytes.",
		},
		func() float64 {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			return float64(stats.HeapSys)
		},
	)

	numGC = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "go_gc_cycles_total",
			Help: "Total number of GC cycles.",
		},
		func() float64 {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			return float64(stats.NumGC)
		},
	)
)

func init() {
	// prometheus.MustRegister(goroutines) // Already registered by Prometheus client library
	prometheus.MustRegister(gcPauseTotal)
	prometheus.MustRegister(heapAlloc)
	prometheus.MustRegister(heapSys)
	prometheus.MustRegister(numGC)
}

// wrapHTTPHandler wraps an http.Handler to work with fasthttp.
func wrapHTTPHandler(h http.Handler) func(ctx *fasthttp.RequestCtx) {
	return func(ctx *fasthttp.RequestCtx) {
		fasthttpadaptor.NewFastHTTPHandler(h)(ctx)
	}
}

// RegisterRoutes wires all API routes onto the provided router.
func RegisterRoutes(r *router.Router) {
	// client auth endpoints
	r.POST("/_sign", backendRoutes.Sign)
	r.POST("/v1/_sign", backendRoutes.Sign)
	r.POST("/v1/sign", backendRoutes.Sign)

	// thread metadata operations
	r.POST("/v1/threads", frontendRoutes.EnqueueCreateThread)
	r.GET("/v1/threads", frontendRoutes.ReadThreadsList)
	r.PUT("/v1/threads/{threadKey}", frontendRoutes.EnqueueUpdateThread)
	r.GET("/v1/threads/{threadKey}", frontendRoutes.ReadThreadItem)
	r.DELETE("/v1/threads/{threadKey}", frontendRoutes.EnqueueDeleteThread)

	// thread message operations
	r.POST("/v1/threads/{threadKey}/messages", frontendRoutes.EnqueueCreateMessage)
	r.GET("/v1/threads/{threadKey}/messages", frontendRoutes.ReadThreadMessages)
	r.GET("/v1/threads/{threadKey}/messages/{id}", frontendRoutes.ReadThreadMessage)
	r.PUT("/v1/threads/{threadKey}/messages/{id}", frontendRoutes.EnqueueUpdateMessage)
	r.DELETE("/v1/threads/{threadKey}/messages/{id}", frontendRoutes.EnqueueDeleteMessage)

	// thread message reactions - REMOVED

	// // helper message endpoints
	// r.POST("/v1/messages", CreateMessage)
	// r.GET("/v1/messages", ListMessages)

	// admin data routes
	r.GET("/admin/health", adminRoutes.Health)
	r.GET("/admin/stats", adminRoutes.Stats)
	r.GET("/admin/keys", adminRoutes.ListKeys)
	r.GET("/admin/keys/{key}", adminRoutes.GetKey)

	// admin hierarchical navigation routes
	r.GET("/admin/users", adminRoutes.ListUsers)
	r.GET("/admin/users/{userId}/threads", adminRoutes.ListUserThreads)
	r.GET("/admin/users/{userId}/threads/{threadKey}/messages", adminRoutes.ListThreadMessages)
	r.GET("/admin/users/{userId}/threads/{threadKey}/messages/{messageKey}", adminRoutes.GetThreadMessage)

	// admin enc routes
	r.POST("/admin/encryption/rotate-thread-dek", adminRoutes.EncryptionRotateThreadDEK)
	r.POST("/admin/encryption/rewrap-deks", adminRoutes.EncryptionRewrapDEKs)
	r.POST("/admin/encryption/encrypt-existing", adminRoutes.EncryptionEncryptExisting)
	r.POST("/admin/encryption/generate-kek", adminRoutes.EncryptionGenerateKEK)

	// admin debug routes
	r.GET("/admin/debug/prometheus", wrapHTTPHandler(promhttp.Handler()))
	r.GET("/admin/debug/pprof/", wrapHTTPHandler(http.HandlerFunc(pprof.Index)))
	r.GET("/admin/debug/pprof/cmdline", wrapHTTPHandler(http.HandlerFunc(pprof.Cmdline)))
	r.GET("/admin/debug/pprof/profile", wrapHTTPHandler(http.HandlerFunc(pprof.Profile)))
	r.GET("/admin/debug/pprof/symbol", wrapHTTPHandler(http.HandlerFunc(pprof.Symbol)))
	r.GET("/admin/debug/pprof/trace", wrapHTTPHandler(http.HandlerFunc(pprof.Trace)))

	// admin job routes
	r.POST("/admin/jobs/purge", adminRoutes.RunRetentionCleanup)
}

// Handler returns the fasthttp handler for the ProgressDB API.
func Handler() fasthttp.RequestHandler {
	r := router.New()
	RegisterRoutes(r)
	return r.Handler
}
