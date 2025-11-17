package ratelimit

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type bucket struct {
	tokens    int
	lastRefill time.Time
	mu        sync.Mutex
}

type RateLimiter struct {
	buckets       map[string]*bucket
	mu            sync.RWMutex
	maxTokens     int
	refillRate    time.Duration
	tokensPerReq  int
	logger        *zap.Logger
	cleanupTicker *time.Ticker
}

type Config struct {
	MaxRequestsPerMinute int
	WindowDuration       time.Duration
	Logger               *zap.Logger
}

func New(cfg Config) *RateLimiter {
	if cfg.MaxRequestsPerMinute == 0 {
		cfg.MaxRequestsPerMinute = 60
	}
	if cfg.WindowDuration == 0 {
		cfg.WindowDuration = time.Minute
	}

	rl := &RateLimiter{
		buckets:       make(map[string]*bucket),
		maxTokens:     cfg.MaxRequestsPerMinute,
		refillRate:    cfg.WindowDuration / time.Duration(cfg.MaxRequestsPerMinute),
		tokensPerReq:  1,
		logger:        cfg.Logger,
		cleanupTicker: time.NewTicker(5 * time.Minute),
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.IP()

		userID := c.Get("X-User-ID")
		if userID != "" {
			key = userID
		}

		if !rl.allow(key) {
			rl.logger.Warn("Rate limit exceeded",
				zap.String("key", key),
				zap.String("ip", c.IP()),
				zap.String("path", c.Path()),
			)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Rate limit exceeded. Please try again later.",
			})
		}

		return c.Next()
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.RLock()
	b, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		b = &bucket{
			tokens:    rl.maxTokens,
			lastRefill: time.Now(),
		}
		rl.buckets[key] = b
		rl.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill)
	tokensToAdd := int(elapsed / rl.refillRate)

	if tokensToAdd > 0 {
		b.tokens = min(rl.maxTokens, b.tokens+tokensToAdd)
		b.lastRefill = now
	}

	if b.tokens >= rl.tokensPerReq {
		b.tokens -= rl.tokensPerReq
		return true
	}

	return false
}

func (rl *RateLimiter) cleanup() {
	for range rl.cleanupTicker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			b.mu.Lock()
			if now.Sub(b.lastRefill) > 10*time.Minute {
				delete(rl.buckets, key)
			}
			b.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Stop() {
	rl.cleanupTicker.Stop()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
