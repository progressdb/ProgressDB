package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"progressdb/pkg/timeutil"
)

type Step struct {
	Name     string  `json:"name"`
	Duration float64 `json:"duration_ms"`
}

type Trace struct {
	Name     string    `json:"name"`
	Start    time.Time `json:"start"`
	Steps    []Step    `json:"steps"`
	TotalMS  float64   `json:"total_ms"`
	lastMark time.Time
	tel      *Telemetry
}

// Telemetry manages async writing of traces to per-op files.
type Telemetry struct {
	dir              string
	mu               sync.Mutex
	files            map[string]*os.File
	buffers          map[string]*bufio.Writer
	traces           chan *Trace
	stopCh           chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
	flushInt         time.Duration
	maxFileSizeBytes int64
	bufferSize       int
	queueCap         int
}

var tel *Telemetry

// Init initializes the global telemetry instance.
func Init(dir string, bufferSize, queueCapacity int, flushInterval time.Duration, maxFileSize int64) {
	tel, _ = New(dir, bufferSize, queueCapacity, flushInterval, maxFileSize)
}

// Track starts a new trace using the global telemetry instance.
func Track(name string) *Trace {
	return tel.Track(name)
}

// Close stops the global telemetry instance.
func Close() {
	if tel != nil {
		tel.Close()
		tel = nil
	}
}

// New creates a new telemetry subsystem with async background writer.
func New(dir string, bufferSize, queueCapacity int, flushInterval time.Duration, maxFileSize int64) (*Telemetry, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	t := &Telemetry{
		dir:              dir,
		files:            make(map[string]*os.File),
		buffers:          make(map[string]*bufio.Writer),
		traces:           make(chan *Trace, queueCapacity),
		stopCh:           make(chan struct{}),
		flushInt:         flushInterval,
		maxFileSizeBytes: maxFileSize, // max file size in bytes
		bufferSize:       bufferSize,
		queueCap:         queueCapacity,
	}
	t.wg.Add(1)
	go t.writerLoop()
	return t, nil
}

// Track starts a new trace that is automatically linked to this telemetry.
func (t *Telemetry) Track(name string) *Trace {
	now := timeutil.Now()
	return &Trace{
		Name:     name,
		Start:    now,
		lastMark: now,
		tel:      t,
	}
}

// Mark records the elapsed duration since last mark.
func (tr *Trace) Mark(label string) {
	now := timeutil.Now()
	delta := now.Sub(tr.lastMark).Seconds() * 1000
	tr.Steps = append(tr.Steps, Step{Name: label, Duration: delta})
	tr.lastMark = now
}

// Finish finalizes the trace and enqueues it for background writing.
// Safe to call multiple times or via defer.
func (tr *Trace) Finish() {
	if tr.tel == nil {
		return
	}
	tr.TotalMS = time.Since(tr.Start).Seconds() * 1000

	// Calculate remaining time not captured in marks
	var sum float64
	for _, s := range tr.Steps {
		sum += s.Duration
	}
	remaining := tr.TotalMS - sum
	if remaining > 0.001 { // threshold to avoid noise
		tr.Steps = append(tr.Steps, Step{Name: "unmarked", Duration: remaining})
	}

	// Optional quick stdout feedback
	// fmt.Printf("Trace: %s (%.2fms)\n", tr.Name, tr.TotalMS)
	// for _, s := range tr.Steps {
	// 	fmt.Printf("  ↳ %-25s %.2fms\n", s.Name, s.Duration)
	// }

	tr.tel.traces <- tr // block if queue full
	tr.tel = nil        // prevent re-send on multiple Finish() calls
}

func (t *Telemetry) writerLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(t.flushInt)
	defer ticker.Stop()

	for {
		select {
		case tr := <-t.traces:
			if tr == nil {
				continue
			}
			data, err := json.Marshal(tr)
			if err != nil {
				continue
			}
			t.mu.Lock()
			b := t.getBufferFor(tr.Name)
			b.Write(data)
			b.WriteByte('\n')
			t.mu.Unlock()

		case <-ticker.C:
			t.mu.Lock()
			for name, b := range t.buffers {
				b.Flush()
				f := t.files[name]
				if fi, err := f.Stat(); err == nil && fi.Size() > t.maxFileSizeBytes {
					// truncate and recreate file when > max size
					f.Close()
					os.Remove(f.Name())
					newF, _ := os.OpenFile(f.Name(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
					t.files[name] = newF
					t.buffers[name] = bufio.NewWriterSize(newF, t.bufferSize)
					fmt.Fprintf(os.Stderr, "telemetry: truncated %s (size exceeded %d bytes)\n", name, t.maxFileSizeBytes)
				}
			}
			t.mu.Unlock()

		case <-t.stopCh:
			t.mu.Lock()
			for _, b := range t.buffers {
				b.Flush()
			}
			for _, f := range t.files {
				f.Sync()
				f.Close()
			}
			t.mu.Unlock()
			return
		}
	}
}

func (t *Telemetry) getBufferFor(op string) *bufio.Writer {
	if b, ok := t.buffers[op]; ok {
		return b
	}
	path := filepath.Join(t.dir, fmt.Sprintf("%s.jsonl", op))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: failed to open %s: %v\n", path, err)
		return bufio.NewWriter(os.Stdout)
	}
	b := bufio.NewWriterSize(f, t.bufferSize)
	t.files[op] = f
	t.buffers[op] = b
	return b
}

// Close stops background writer and flushes all remaining data.
func (t *Telemetry) Close() {
	t.stopOnce.Do(func() {
		close(t.stopCh)
		t.wg.Wait()
	})
}
