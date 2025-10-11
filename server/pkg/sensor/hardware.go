package sensor

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// Snapshot contains a lightweight view of system resources useful for
// adaptive batching and throttling decisions. Fields are best-effort and may
// be zero on unsupported platforms.
type Snapshot struct {
	Timestamp time.Time

	// CPU utilization [0..1] across all CPUs (best-effort)
	CPUUtil float64

	// Memory in bytes
	MemTotal uint64
	MemUsed  uint64

	// Disk free/total in bytes for the filesystem where the process runs
	DiskTotal uint64
	DiskFree  uint64

	// Lightweight I/O counters (best-effort)
	DiskReadBytes  uint64
	DiskWriteBytes uint64

	// Network counters (best-effort)
	NetRxBytes uint64
	NetTxBytes uint64
}

// ThrottleRequest is an optimistic signal emitted by components that want
// others to throttle down or release resources. It's advisory only.
type ThrottleRequest struct {
	// Who is requesting (optional)
	Source string
	// Reason is a short string describing the request
	Reason string
	// Severity [0..1] where 1 is most urgent
	Severity float64
	// Optional payload for future extension
	Payload map[string]string
}

// Sensor polls host resources and exposes a current Snapshot. It also
// provides a simple pub/sub for optimistic throttle requests.
type Sensor struct {
	mu   sync.RWMutex
	snap Snapshot
	// procfs reserved for future richer sampling on Linux.
	// kept as a placeholder so we can add platform-specific sampling later.
	_reserved interface{}
	interval  time.Duration

	// throttle handlers
	thMu     sync.RWMutex
	handlers []func(ThrottleRequest)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSensor creates a sensor that polls every interval. If procfs is
// unavailable (non-Linux), the sensor will still provide runtime stats.
func NewSensor(interval time.Duration) *Sensor {
	s := &Sensor{interval: interval}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Start begins background polling. Call Stop to terminate.
func (s *Sensor) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		// warm initial sample
		s.sample()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.sample()
			}
		}
	}()
}

// Stop stops background polling and waits for workers to exit.
func (s *Sensor) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Snapshot returns the most recent snapshot (fast, copy).
func (s *Sensor) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// RegisterThrottleHandler registers a callback to receive optimistic
// throttle requests. Handlers are invoked asynchronously.
func (s *Sensor) RegisterThrottleHandler(h func(ThrottleRequest)) {
	s.thMu.Lock()
	defer s.thMu.Unlock()
	s.handlers = append(s.handlers, h)
}

// SendThrottle emits an optimistic throttle request to registered handlers.
// This is non-blocking and best-effort.
func (s *Sensor) SendThrottle(req ThrottleRequest) {
	s.thMu.RLock()
	handlers := append([]func(ThrottleRequest){}, s.handlers...)
	s.thMu.RUnlock()
	for _, h := range handlers {
		go func(cb func(ThrottleRequest)) {
			// run with a small timeout to avoid runaway handlers
			done := make(chan struct{})
			go func() {
				cb(req)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(250 * time.Millisecond):
			}
		}(h)
	}
}

// sample collects best-effort metrics and updates the current snapshot.
func (s *Sensor) sample() {
	snap := Snapshot{
		Timestamp: time.Now(),
	}

	// Capture memory statistics.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	snap.MemTotal = memStats.Sys
	snap.MemUsed = memStats.Alloc

	// Provide a basic (placeholder) CPU utilization estimate.
	cpuCount := runtime.NumCPU()
	if cpuCount > 0 {
		// This is not true CPU utilization—just a signal that CPUs are present.
		snap.CPUUtil = 1.0
	}

	// Disk* and Net* fields remain zero if not available on this platform.

	// Atomically update the snapshot.
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}
