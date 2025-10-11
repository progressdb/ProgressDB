package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"progressdb/pkg/state"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Minimal, low-overhead request telemetry designed for local usage.
// - By default only slow requests are logged (see slowThreshold).
// - Per-request spans are only recorded when a request is sampled (very low default sampling).

type ctxKeyType struct{}

var (
	writerOnce sync.Once
	writerCh   chan []byte
	requestCtr uint64
	spanCtr    uint64
	// defaults are provided by central config at startup; zero disables tracing until set
	sampleRate    = 0.0
	slowThreshold = 0 * time.Millisecond
)

// Span is a simple span relative to request start (milliseconds)
type Span struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Op       string `json:"op"`
	StartMs  int64  `json:"start_ms"`
	Duration int64  `json:"duration_ms"`
}

// Telemetry holds the per-request trace and metadata. startTime is not exported.
type Telemetry struct {
	RequestID string `json:"request_id"`
	Op        string `json:"op"`
	Path      string `json:"path"`
	StartMs   int64  `json:"start_ms"`
	Duration  int64  `json:"duration_ms"`
	Status    int    `json:"status"`
	Spans     []Span `json:"spans,omitempty"`

	// internal
	startTime time.Time
	mu        sync.Mutex
	// span stack for parent linkage
	spanStack []string
}

// initWriter lazily starts a background writer that appends JSON lines to logs/telemetry.jsonl.
func initWriter() {
	writerCh = make(chan []byte, 1024)
	go func() {
		// try to create state dirs if missing; best-effort
		dbDir := filepath.Join("db", "state", "telemetry")
		stdDir := filepath.Join("standard", "state", "telemetry")
		if state.PathsVar.State != "" {
			dbDir = filepath.Join(state.PathsVar.State, "telemetry")
			stdDir = filepath.Join(state.PathsVar.State, "telemetry")
		} else if root := state.ArtifactRoot(); root != "" {
			dbDir = filepath.Join(root, "db", "state", "telemetry")
			stdDir = filepath.Join(root, "standard", "state", "telemetry")
		}
		_ = os.MkdirAll(dbDir, 0o755)
		_ = os.MkdirAll(stdDir, 0o755)

		f1, err1 := os.OpenFile(filepath.Join(dbDir, "telemetry.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		var f2 *os.File
		var err2 error
		f2, err2 = os.OpenFile(filepath.Join(stdDir, "telemetry.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

		// if both fail, bail out silently
		if err1 != nil && err2 != nil {
			return
		}
		// ensure files that opened get closed
		if err1 == nil {
			defer f1.Close()
		}
		if err2 == nil {
			defer f2.Close()
		}

		for b := range writerCh {
			line := append(b, '\n')
			if err1 == nil {
				_, _ = f1.Write(line)
			}
			if err2 == nil {
				_, _ = f2.Write(line)
			}
		}
	}()
}

// EmitDiagnostic collects runtime diagnostics (goroutine stacks, memstats)
// and writes a JSON-line diagnostic record to the telemetry writer. This is
// best-effort and will not block the caller.
func EmitDiagnostic(name string) {
	// lightweight runtime diagnostic: avoid collecting full memstats and
	// goroutine stacks because those operations are expensive and can
	// significantly slow request handling when emitted frequently.
	rec := map[string]interface{}{
		"type":          "diagnostic",
		"name":          name,
		"ts":            time.Now().UnixNano() / 1e6,
		"num_goroutine": runtime.NumGoroutine(),
		"num_cpu":       runtime.NumCPU(),
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	writerOnce.Do(initWriter)
	select {
	case writerCh <- b:
	default:
	}
}

// Middleware wraps the provided handler and records request timing and sampled spans.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sampled := shouldSample(r)

		var tel *Telemetry
		// Force sampling for create_message endpoint so we always get summaries
		pathNorm := normalizePath(r.URL.Path)
		if pathNorm == "v1_messages" || strings.Contains(r.URL.Path, "/v1/messages") {
			sampled = true
		}

		if sampled {
			id := ""
			op := r.Header.Get("X-Operation")
			if op == "" {
				// fallback to request path for meaningful op when header not provided
				op = r.URL.Path
			}
			tel = &Telemetry{
				RequestID: id,
				Op:        op,
				Path:      normalizePath(r.URL.Path),
				startTime: start,
				StartMs:   start.UnixNano() / 1e6,
			}
			// create a root span representing this request's top-level op
			rootID := genSpanID()
			rootSpan := Span{ID: rootID, Op: tel.Op, StartMs: 0}
			tel.Spans = append(tel.Spans, rootSpan)
			tel.spanStack = append(tel.spanStack, rootID)
			// attach to context for instrumentation points
			ctx := context.WithValue(r.Context(), ctxKeyType{}, tel)
			r = r.WithContext(ctx)
		}

		// capture status
		srw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(srw, r)

		dur := time.Since(start)
		if tel != nil {
			tel.mu.Lock()
			tel.Status = srw.status
			tel.Duration = dur.Milliseconds()
			// render text block and enqueue
			b := renderTelemetryText(tel)
			tel.mu.Unlock()
			// if this request is slow, also emit a diagnostic JSON record
			if dur > slowThreshold {
				EmitDiagnostic("slow_request_" + normalizePath(r.URL.Path))
			}
			writerOnce.Do(initWriter)
			select {
			case writerCh <- b:
			default:
				// drop if channel full to avoid blocking
			}
			return
		}

		// For non-sampled requests, emit diagnostics when the request is
		// slower than the configured threshold so we capture runtime state.
		if dur > slowThreshold {
			EmitDiagnostic("slow_request_" + normalizePath(r.URL.Path))
		}
	})
}

// From a context, StartSpan returns an end function. If telemetry isn't enabled for the request,
// StartSpan returns a no-op end function (very low overhead).
func StartSpan(ctx context.Context, name string) func() {
	v := ctx.Value(ctxKeyType{})
	if v == nil {
		return func() {}
	}
	tel, ok := v.(*Telemetry)
	if !ok {
		return func() {}
	}
	startRel := time.Since(tel.startTime).Milliseconds()
	id := genSpanID()
	parent := ""

	tel.mu.Lock()
	if len(tel.spanStack) > 0 {
		parent = tel.spanStack[len(tel.spanStack)-1]
	}
	s := Span{ID: id, ParentID: parent, Op: name, StartMs: startRel}
	tel.Spans = append(tel.Spans, s)
	tel.spanStack = append(tel.spanStack, id)
	idx := len(tel.Spans) - 1
	tel.mu.Unlock()

	return func() {
		endRel := time.Since(tel.startTime).Milliseconds()
		tel.mu.Lock()
		if idx < len(tel.Spans) {
			tel.Spans[idx].Duration = endRel - tel.Spans[idx].StartMs
		}
		// pop stack
		if len(tel.spanStack) > 0 {
			tel.spanStack = tel.spanStack[:len(tel.spanStack)-1]
		}
		tel.mu.Unlock()
	}
}

// WithRoot ensures a telemetry root span exists on the context. If the
// context already has telemetry (e.g. middleware), it returns the same
// context and a child-span end function. If not, it creates a temporary
// telemetry root for the duration of the returned end function and writes
// the trace when ended.
func WithRoot(ctx context.Context, op string) (context.Context, func()) {
	v := ctx.Value(ctxKeyType{})
	if v == nil {
		// create a new telemetry for this context
		now := time.Now()
		tel := &Telemetry{
			RequestID: "",
			Op:        op,
			Path:      "",
			startTime: now,
			StartMs:   now.UnixNano() / 1e6,
		}
		// create root span
		rootID := genSpanID()
		rootSpan := Span{ID: rootID, Op: tel.Op, StartMs: 0}
		tel.Spans = append(tel.Spans, rootSpan)
		tel.spanStack = append(tel.spanStack, rootID)
		ctx2 := context.WithValue(ctx, ctxKeyType{}, tel)
		end := func() {
			dur := time.Since(now)
			tel.mu.Lock()
			tel.Duration = dur.Milliseconds()
			tel.Status = 0
			b := renderTelemetryText(tel)
			tel.mu.Unlock()
			writerOnce.Do(initWriter)
			select {
			case writerCh <- b:
			default:
			}
		}
		return ctx2, end
	}
	// already have telemetry - just return a child span ender
	return ctx, StartSpan(ctx, op)
}

// SetRequestOp allows a handler to override the top-level operation name for
// the current request telemetry. It will also update the root span op when
// present.
func SetRequestOp(ctx context.Context, op string) {
	v := ctx.Value(ctxKeyType{})
	if v == nil {
		return
	}
	tel, ok := v.(*Telemetry)
	if !ok {
		return
	}
	tel.mu.Lock()
	defer tel.mu.Unlock()
	tel.Op = op
	if len(tel.Spans) > 0 {
		// assume first span is root
		tel.Spans[0].Op = op
	}
}

// renderTelemetryText renders a sampled Telemetry as an indented text block.
func renderTelemetryText(t *Telemetry) []byte {
	// Render a simple parent + children list: parent has op and normalized path,
	// children are printed as lines with label and duration only.
	if len(t.Spans) == 0 {
		return []byte("")
	}
	rootID := t.Spans[0].ID

	// build children map
	children := make(map[string][]Span)
	for _, sp := range t.Spans {
		children[sp.ParentID] = append(children[sp.ParentID], sp)
	}

	var b strings.Builder
	// header: human-friendly op label and normalized path
	fmt.Fprintf(&b, "PARENT op=%s path=%s duration_ms=%d status=%d\n", t.Op, t.Path, t.Duration, t.Status)

	// recursive print of subtree starting under rootID
	var printChildren func(parent string, depth int)
	printChildren = func(parent string, depth int) {
		list := children[parent]
		sort.SliceStable(list, func(i, j int) bool { return list[i].StartMs < list[j].StartMs })
		for _, sp := range list {
			indent := strings.Repeat("  ", depth)
			fmt.Fprintf(&b, "%s%s duration_ms=%d\n", indent, sp.Op, sp.Duration)
			printChildren(sp.ID, depth+1)
		}
	}

	printChildren(rootID, 1)
	b.WriteString("\n")
	return []byte(b.String())
}

// Helper: basic sampling decision. Also supports forcing sampling via header `X-Debug-Telemetry: 1`.
func shouldSample(r *http.Request) bool {
	if r.Header.Get("X-Debug-Telemetry") == "1" {
		return true
	}
	// very cheap check: use an atomic counter to sample deterministically
	if sampleRate <= 0 {
		return false
	}
	// convert sampleRate to a simple 1-in-N sampling when sampleRate is small
	// e.g. 0.001 -> 1 in 1000
	denom := int64(1 / sampleRate)
	if denom <= 1 {
		return true
	}
	n := int64(atomic.AddUint64(&requestCtr, 1))
	return (n % denom) == 0
}

func genSpanID() string {
	n := atomic.AddUint64(&spanCtr, 1)
	return "s-" + fmtUint64(n)
}

// SetSampleRate sets the approximate sampling rate for full traces (0..1).
// A rate of 0 disables full tracing (only slow requests will be logged).
func SetSampleRate(r float64) {
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	sampleRate = r
}

// SetSlowThreshold sets the duration above which non-sampled requests get a lightweight log.
func SetSlowThreshold(d time.Duration) {
	if d <= 0 {
		d = 0
	}
	slowThreshold = d
}

// --- Simple, context-less helpers ---
// These helpers provide a lightweight, request-agnostic way to record spans
// during migration away from context-bound telemetry. They write a small JSON
// line with the span name and duration when the returned end function is
// invoked. They are intentionally minimal and do not attempt to build full
// parent/child traces.

// StartSpanNoCtx starts a simple span that is not associated with any
// request context. It returns an end function to record the span duration.
func StartSpanNoCtx(name string) func() {
	start := time.Now()
	return func() {
		dur := time.Since(start).Milliseconds()
		rec := map[string]interface{}{
			"type":          "span",
			"name":          name,
			"ts":            time.Now().UnixNano() / 1e6,
			"duration_ms":   dur,
			"num_goroutine": runtime.NumGoroutine(),
		}
		b, err := json.Marshal(rec)
		if err != nil {
			return
		}
		writerOnce.Do(initWriter)
		select {
		case writerCh <- b:
		default:
		}
	}
}

// WithRootNoCtx creates a short-lived root span (no-op implementation of a
// request root) and returns an end function that will write a summary record
// when invoked.
func WithRootNoCtx(op string) func() {
	start := time.Now()
	return func() {
		dur := time.Since(start).Milliseconds()
		rec := map[string]interface{}{
			"type":        "root",
			"op":          op,
			"ts":          time.Now().UnixNano() / 1e6,
			"duration_ms": dur,
		}
		b, err := json.Marshal(rec)
		if err != nil {
			return
		}
		writerOnce.Do(initWriter)
		select {
		case writerCh <- b:
		default:
		}
	}
}

// SetRequestOpNoCtx is a no-op placeholder used during migration when a
// request-scoped telemetry root is not available. It exists for symmetry
// with SetRequestOp but does not modify any state.
func SetRequestOpNoCtx(op string) {}

// small helper to avoid importing fmt for a single use
func fmtUint64(v uint64) string {
	// simple base10 conversion
	buf := make([]byte, 0, 20)
	if v == 0 {
		return "0"
	}
	for v > 0 {
		d := byte(v % 10)
		buf = append(buf, byte('0')+d)
		v /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// normalizePath converts a request path into a compact label used as the
// parent path identifier: removes leading slash, replaces '/' with '_', and
// collapses non-alphanumeric to underscores.
func normalizePath(p string) string {
	if p == "/" || p == "" {
		return "root"
	}
	// trim leading/trailing slash
	if strings.HasPrefix(p, "/") {
		p = p[1:]
	}
	if strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	// replace '/' with '_'
	p = strings.ReplaceAll(p, "/", "_")
	// collapse non-alnum to underscore
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "root"
	}
	return out
}

// statusRecorder captures the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
