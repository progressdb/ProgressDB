package httpx

import (
	"net/http"
)

// NetHTTPAdapter adapts an httpx.HandlerFunc into a standard net/http handler.
func NetHTTPAdapter(h HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := &Request{
			Ctx:        r.Context(),
			Method:     r.Method,
			Path:       r.URL.Path,
			Header:     r.Header.Clone(),
			Body:       r.Body,
			RemoteAddr: r.RemoteAddr,
			Raw:        r,
		}

		rw := &netHTTPResponseWriter{w: w, header: make(http.Header)}
		// copy existing headers from underlying writer
		for k, v := range w.Header() {
			rw.header[k] = append([]string(nil), v...)
		}

		h(rw, req)
		// ensure body is closed if handler did not close it
		if req.Body != nil {
			_ = req.Body.Close()
		}
	})
}

type netHTTPResponseWriter struct {
	w      http.ResponseWriter
	header http.Header
	status int
}

func (r *netHTTPResponseWriter) Header() http.Header { return r.header }

func (r *netHTTPResponseWriter) WriteHeader(status int) {
	r.status = status
	// copy headers to underlying writer and call WriteHeader
	for k, v := range r.header {
		r.w.Header()[k] = append([]string(nil), v...)
	}
	r.w.WriteHeader(status)
}

func (r *netHTTPResponseWriter) Write(b []byte) (int, error) {
	// ensure headers flushed
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.w.Write(b)
}
