package config

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.Mutex
	rate     rate.Limit
	burst    int
	ttl      time.Duration
	stopCh   chan struct{}
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(cfg *AppConfig) *RateLimiter {
	r := rate.Every(time.Duration(cfg.TempCodeRateLimitSeconds) * time.Second)
	b := 1

	ttl := time.Duration(cfg.TempCodeRateLimitSeconds)*time.Second + (5 * time.Second)

	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     r,
		burst:    b,
		ttl:      ttl,
		stopCh:   make(chan struct{}),
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[key] = &visitor{limiter, time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	limiter := rl.GetLimiter(key)
	reservation := limiter.Reserve()
	if !reservation.OK() {
		return false, 0
	}

	delay := reservation.Delay()
	if delay == 0 {
		return true, 0
	}

	reservation.Cancel()
	return false, delay
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			for key, v := range rl.visitors {
				if time.Since(v.lastSeen) > rl.ttl {
					delete(rl.visitors, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}
