package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterPool is a simple per-identifier token-bucket pool backed by
// golang.org/x/time/rate.Limiter values. A limiter is created on first use
// for an identifier key (the identifier is the API key when present or the
// client IP when no API key is provided).
//
// Behavior:
// - Each identifier has an independent token bucket created with the
//   configured RPS (tokens per second) and Burst capacity.
// - The middleware uses non-blocking Allow() to immediately accept or
//   reject requests; when Allow() returns false the request is rejected
//   with HTTP 429 (Too Many Requests).
// - Defaults: when the configured RPS or Burst are <= 0 we fall back to
//   sensible defaults. The RPS default is 100 requests/sec and the Burst
//   default is 100 tokens (to allow short bursts).
//
// Notes / Limitations:
// - There is currently no eviction for the map of limiters; a long tail of
//   distinct keys (many IPs or ephemeral API keys) can grow memory usage.
//   Consider adding TTL/LRU eviction for production workloads.
// - The policy is global per-identifier. If you need per-route or per-role
//   limits, add that logic in the gateway before calling Allow().

type limiterEntry struct {
	l        *rate.Limiter
	lastSeen time.Time
}

type limiterPool struct {
	mu            sync.Mutex
	m             map[string]*limiterEntry
	cfg           SecConfig
	startCleanup  sync.Once
	ttl           time.Duration
	cleanupPeriod time.Duration
}

// get returns the limiter for a key, creating one if missing. It also
// updates the entry's lastSeen timestamp. The pool lazily starts a
// background cleanup goroutine on first use.
func (p *limiterPool) get(key string) *rate.Limiter {
	// ensure cleanup goroutine is running
	p.startCleanup.Do(func() {
		// defaults: TTL 10 minutes, cleanup every 1 minute
		if p.ttl == 0 {
			p.ttl = 10 * time.Minute
		}
		if p.cleanupPeriod == 0 {
			p.cleanupPeriod = time.Minute
		}
		go p.cleanupLoop()
	})

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = make(map[string]*limiterEntry)
	}
	if e, ok := p.m[key]; ok {
		e.lastSeen = time.Now()
		return e.l
	}

	rps := p.cfg.RPS
	burst := p.cfg.Burst
	l := rate.NewLimiter(rate.Limit(rps), burst)
	p.m[key] = &limiterEntry{l: l, lastSeen: time.Now()}
	return l
}

// Allow checks the limiter for key and returns whether the request is allowed.
// It updates the entry lastSeen when present/created.
func (p *limiterPool) Allow(key string) bool {
	return p.get(key).Allow()
}

// cleanupLoop periodically removes entries that have not been seen within
// the configured TTL to prevent unbounded growth of the limiter map.
func (p *limiterPool) cleanupLoop() {
	ticker := time.NewTicker(p.cleanupPeriod)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-p.ttl)
		p.mu.Lock()
		for k, e := range p.m {
			if e.lastSeen.Before(cutoff) {
				delete(p.m, k)
			}
		}
		p.mu.Unlock()
	}
}
