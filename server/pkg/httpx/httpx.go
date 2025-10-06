package httpx

import (
	"context"
	"io"
	"net/http"
)

// Request is the unified request representation used by handlers.
// Handlers should prefer using Request.Ctx for cancellations/values.
type Request struct {
	Ctx        context.Context
	Method     string
	Path       string
	Header     http.Header
	Body       io.ReadCloser
	RemoteAddr string
	// Raw holds the underlying transport-specific request object
	// (e.g. *http.Request or *fasthttp.RequestCtx) for escape hatches.
	Raw interface{}
}

// ResponseWriter is a small subset of http.ResponseWriter semantics
// that we require from adapters.
type ResponseWriter interface {
	Header() http.Header
	Write([]byte) (int, error)
	WriteHeader(status int)
}

// HandlerFunc is the application handler signature used across adapters.
type HandlerFunc func(w ResponseWriter, r *Request)
