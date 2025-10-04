package auth

import (
	"sync"

	"golang.org/x/time/rate"
)

type limiterPool struct {
	mu  sync.Mutex
	m   map[string]*rate.Limiter
	cfg SecConfig
}

func (p *limiterPool) get(key string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = make(map[string]*rate.Limiter)
	}
	if l, ok := p.m[key]; ok {
		return l
	}
	rps := p.cfg.RPS
	if rps <= 0 {
		rps = 5
	}
	burst := p.cfg.Burst
	if burst <= 0 {
		burst = 10
	}
	l := rate.NewLimiter(rate.Limit(rps), burst)
	p.m[key] = l
	return l
}

func (p *limiterPool) Allow(key string) bool {
	// Use per-second rate; limiter handles clocks
	return p.get(key).Allow()
}
