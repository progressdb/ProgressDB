package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"progressdb/pkg/state"
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
	writerOnce    sync.Once
	writerCh      chan []byte
	requestCtr    uint64
	spanCtr       uint64
	sampleRate    = 0.001 // 0.1% default sampling for full traces (very low)
	slowThreshold = 200 * time.Millisecond
)

// Span is a simple span relative to request start (milliseconds)
type Span struct {
	ID       string                 `json:"id"`
	ParentID string                 `json:"parent_id,omitempty"`
	Op       string                 `json:"op"`
	StartMs  int64                  `json:"start_ms"`
	Duration int64                  `json:"duration_ms"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// Telemetry holds the per-request trace and metadata. startTime is not exported.
type Telemetry struct {
	RequestID string `json:"request_id"`
	Op        string `json:"op"`
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

// Middleware wraps the provided handler and records request timing and sampled spans.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// generate a request id for both sampled and non-sampled flows so slow logs
		// and any later instrumentation can reference the same id.
		reqID := genRequestID()
		sampled := shouldSample(r)

		var tel *Telemetry
		if sampled {
			id := reqID
			op := r.Header.Get("X-Operation")
			if op == "" {
				// fallback to request path for meaningful op when header not provided
				op = r.URL.Path
			}
			tel = &Telemetry{
				RequestID: id,
				Op:        op,
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
			writerOnce.Do(initWriter)
			select {
			case writerCh <- b:
			default:
				// drop if channel full to avoid blocking
			}
			return
		}

		// not sampled: only log slow requests as lightweight text
		if dur > slowThreshold {
			// op fallback to header or path
			op := r.Header.Get("X-Operation")
			if op == "" {
				op = r.URL.Path
			}
			rec := map[string]interface{}{
				"request_id":  reqID,
				"op":          op,
				"duration_ms": dur.Milliseconds(),
				"status":      srw.status,
			}
			b := renderSlowText(rec)
			writerOnce.Do(initWriter)
			select {
			case writerCh <- b:
			default:
			}
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

// SetSpanData attaches a key/value to the currently active span for the
// request (no-op if telemetry isn't enabled or no active span).
func SetSpanData(ctx context.Context, key string, value interface{}) {
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
	if len(tel.spanStack) == 0 {
		return
	}
	top := tel.spanStack[len(tel.spanStack)-1]
	// find span by id from end
	for i := len(tel.Spans) - 1; i >= 0; i-- {
		if tel.Spans[i].ID == top {
			if tel.Spans[i].Data == nil {
				tel.Spans[i].Data = make(map[string]interface{})
			}
			tel.Spans[i].Data[key] = value
			return
		}
	}
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
	// If this trace is about create_message, emit a compact single-line
	// summary to keep telemetry simple and fast.
	for _, sp := range t.Spans {
		if strings.Contains(sp.Op, "create_message") {
			// emit single-line request summary
			return []byte(fmt.Sprintf("REQ %s op=%s duration_ms=%d status=%d\n", t.RequestID, "create_message", t.Duration, t.Status))
		}
	}

	// otherwise, fallback to full indented format
	var b strings.Builder
	// top header
	fmt.Fprintf(&b, "REQUEST %s op=%s start_ms=%d duration_ms=%d status=%d\n", t.RequestID, t.Op, t.StartMs, t.Duration, t.Status)

	// build children map
	children := make(map[string][]Span)
	for _, sp := range t.Spans {
		children[sp.ParentID] = append(children[sp.ParentID], sp)
	}

	// recursive print
	var printSpan func(id string, depth int)
	printSpan = func(id string, depth int) {
		list := children[id]
		// sort by start time (stable) to get consistent ordering
		sort.SliceStable(list, func(i, j int) bool { return list[i].StartMs < list[j].StartMs })
		for _, sp := range list {
			indent := strings.Repeat("  ", depth)
			// compact data
			dataStr := ""
			if len(sp.Data) > 0 {
				var parts []string
				for k, v := range sp.Data {
					parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				}
				dataStr = " data=" + strings.Join(parts, ",")
			}
			fmt.Fprintf(&b, "%s- %s id=%s start_ms=%d duration_ms=%d%s\n", indent, sp.Op, sp.ID, sp.StartMs, sp.Duration, dataStr)
			// recurse into children
			printSpan(sp.ID, depth+1)
		}
	}

	// start from root (parent == "")
	printSpan("", 1)
	// trailing blank line to separate requests
	b.WriteString("\n")
	return []byte(b.String())
}

// renderSlowText renders the compact single-line slow request record.
func renderSlowText(rec map[string]interface{}) []byte {
	var b strings.Builder
	rid, _ := rec["request_id"].(string)
	op, _ := rec["op"].(string)
	dur := rec["duration_ms"]
	status := rec["status"]
	fmt.Fprintf(&b, "SLOW %s op=%s duration_ms=%v status=%v\n", rid, op, dur, status)
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

func genRequestID() string {
	n := atomic.AddUint64(&requestCtr, 1)
	return "r-" + time.Now().Format("20060102T150405") + "-" + fmtUint64(n)
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

// statusRecorder captures the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
