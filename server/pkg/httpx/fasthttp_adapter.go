package httpx

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/valyala/fasthttp"
)

// FastHTTPAdapter adapts an httpx.HandlerFunc into a fasthttp.RequestHandler.
// It creates a request context with cancellation and exposes it via Request.Ctx.
func FastHTTPAdapter(h HandlerFunc) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// create a cancellable context for this request
		cctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// build headers
		hdr := make(http.Header)
		ctx.Request.Header.VisitAll(func(k, v []byte) {
			key := string(k)
			hdr[key] = append(hdr[key], string(v))
		})

		// create a ReadCloser over the request body (PostBody) — small copy
		bodyBytes := ctx.PostBody()
		var body io.ReadCloser
		if len(bodyBytes) > 0 {
			body = io.NopCloser(bytes.NewReader(bodyBytes))
		} else {
			body = io.NopCloser(bytes.NewReader(nil))
		}

		req := &Request{
			Ctx:        cctx,
			Method:     string(ctx.Method()),
			Path:       string(ctx.Path()),
			Header:     hdr,
			Body:       body,
			RemoteAddr: ctx.RemoteAddr().String(),
			Raw:        ctx,
		}

		rw := &fastHTTPResponseWriter{ctx: ctx, header: make(http.Header)}
		// populate initial headers from response
		ctx.Response.Header.VisitAll(func(k, v []byte) {
			rw.header[string(k)] = append(rw.header[string(k)], string(v))
		})

		h(rw, req)

		// ensure request body closed
		if req.Body != nil {
			_ = req.Body.Close()
		}
	}
}

type fastHTTPResponseWriter struct {
	ctx    *fasthttp.RequestCtx
	header http.Header
	status int
}

func (f *fastHTTPResponseWriter) Header() http.Header { return f.header }

func (f *fastHTTPResponseWriter) WriteHeader(status int) {
	f.status = status
	// copy headers into fasthttp response header
	for k, vals := range f.header {
		for _, v := range vals {
			f.ctx.Response.Header.Add(k, v)
		}
	}
	f.ctx.SetStatusCode(status)
}

func (f *fastHTTPResponseWriter) Write(b []byte) (int, error) {
	if f.status == 0 {
		f.WriteHeader(http.StatusOK)
	}
	return f.ctx.Write(b)
}
