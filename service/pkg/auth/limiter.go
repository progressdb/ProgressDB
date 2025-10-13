package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Per-key rate limiter pool.
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

// get limiter for key, create if missing; start cleanup once
func (p *limiterPool) get(key string) *rate.Limiter {
	p.startCleanup.Do(func() {
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

	l := rate.NewLimiter(rate.Limit(p.cfg.RPS), p.cfg.Burst)
	p.m[key] = &limiterEntry{l: l, lastSeen: time.Now()}
	return l
}

// allow returns true if the current request is allowed.
func (p *limiterPool) Allow(key string) bool {
	return p.get(key).Allow()
}

// cleanupLoop removes limiters unused > TTL.
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
