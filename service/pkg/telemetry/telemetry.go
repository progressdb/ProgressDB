package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	strategy RotationStrategy
}

// Telemetry manages async writing of traces to per-op files.
type RotationStrategy int

const (
	RotationStrategyTruncate RotationStrategy = iota
	RotationStrategyPurge
)

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
	rotationStrategy RotationStrategy
}

var tel *Telemetry

// Init initializes the global telemetry instance.
func Init(dir string, bufferSize, queueCapacity int, flushInterval time.Duration, maxFileSize int64) {
	tel, _ = New(dir, bufferSize, queueCapacity, flushInterval, maxFileSize, RotationStrategyPurge)
}

// Track starts a new trace using the global telemetry instance.
func Track(name string) *Trace {
	return tel.Track(name)
}

// TrackWithStrategy starts a new trace with specific rotation strategy.
func TrackWithStrategy(name string, strategy RotationStrategy) *Trace {
	return tel.TrackWithStrategy(name, strategy)
}

// Close stops the global telemetry instance.
func Close() {
	if tel != nil {
		tel.Close()
		tel = nil
	}
}

// InitWithStrategy initializes the global telemetry instance with a specific rotation strategy.
func InitWithStrategy(dir string, bufferSize, queueCapacity int, flushInterval time.Duration, maxFileSize int64, strategy RotationStrategy) {
	tel, _ = New(dir, bufferSize, queueCapacity, flushInterval, maxFileSize, strategy)
}

// New creates a new telemetry subsystem with async background writer.
func New(dir string, bufferSize, queueCapacity int, flushInterval time.Duration, maxFileSize int64, strategy RotationStrategy) (*Telemetry, error) {
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
		rotationStrategy: strategy,
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
		strategy: t.rotationStrategy, // default to telemetry's strategy
	}
}

// TrackWithStrategy starts a new trace with specific rotation strategy.
func (t *Telemetry) TrackWithStrategy(name string, strategy RotationStrategy) *Trace {
	now := timeutil.Now()
	return &Trace{
		Name:     name,
		Start:    now,
		lastMark: now,
		tel:      t,
		strategy: strategy,
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
			b := t.getBufferFor(tr.Name, tr.strategy)
			b.Write(data)
			b.WriteByte('\n')
			t.mu.Unlock()

		case <-ticker.C:
			t.mu.Lock()
			for key, b := range t.buffers {
				b.Flush()
				f := t.files[key]
				if fi, err := f.Stat(); err == nil && fi.Size() > t.maxFileSizeBytes {
					// extract strategy from key (format: "operation_strategy")
					parts := strings.Split(key, "_")
					if len(parts) < 2 {
						continue
					}
					strategyStr := parts[len(parts)-1]
					var strategy RotationStrategy
					if strategyStr == "1" {
						strategy = RotationStrategyPurge
					} else {
						strategy = RotationStrategyTruncate
					}

					switch strategy {
					case RotationStrategyPurge:
						// purge file completely when > max size
						f.Close()
						os.Remove(f.Name())
						delete(t.files, key)
						delete(t.buffers, key)
						fmt.Fprintf(os.Stderr, "telemetry: purged %s (size exceeded %d bytes)\n", key, t.maxFileSizeBytes)
					case RotationStrategyTruncate:
						// truncate and recreate file when > max size
						f.Close()
						os.Remove(f.Name())
						newF, _ := os.OpenFile(f.Name(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
						t.files[key] = newF
						t.buffers[key] = bufio.NewWriterSize(newF, t.bufferSize)
						fmt.Fprintf(os.Stderr, "telemetry: truncated %s (size exceeded %d bytes)\n", key, t.maxFileSizeBytes)
					}
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

func (t *Telemetry) getBufferFor(op string, strategy RotationStrategy) *bufio.Writer {
	key := fmt.Sprintf("%s_%v", op, strategy) // unique key per operation+strategy
	if b, ok := t.buffers[key]; ok {
		return b
	}
	path := filepath.Join(t.dir, fmt.Sprintf("%s_%v.jsonl", op, strategy))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: failed to open %s: %v\n", path, err)
		return bufio.NewWriter(os.Stdout)
	}
	b := bufio.NewWriterSize(f, t.bufferSize)
	t.files[key] = f
	t.buffers[key] = b
	return b
}

// Close stops background writer and flushes all remaining data.
func (t *Telemetry) Close() {
	t.stopOnce.Do(func() {
		close(t.stopCh)
		t.wg.Wait()
	})
}
