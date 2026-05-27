package resilience

import (
	"context"
	"sync"
	"time"

	"github.com/team-dandelion/quickgo/gerr"
)

// RateLimiter 限流器接口
type RateLimiter interface {
	// Allow 检查是否允许请求通过
	Allow() bool
	// AllowN 检查是否允许 n 个请求通过
	AllowN(n int) bool
	// Wait 阻塞等待直到请求被允许
	Wait(ctx context.Context) error
	// WaitN 阻塞等待直到 n 个请求被允许
	WaitN(ctx context.Context, n int) error
}

// TokenBucketLimiter 令牌桶限流器
type TokenBucketLimiter struct {
	mu         sync.Mutex
	tokens     float64   // 当前令牌数
	maxTokens  float64   // 最大令牌数
	refillRate float64   // 每秒填充令牌数
	lastRefill time.Time // 上次填充时间
}

// TokenBucketConfig 令牌桶配置
type TokenBucketConfig struct {
	MaxTokens  int     // 桶最大容量
	RefillRate float64 // 每秒填充令牌数
}

// NewTokenBucketLimiter 创建令牌桶限流器
func NewTokenBucketLimiter(config TokenBucketConfig) *TokenBucketLimiter {
	if config.MaxTokens <= 0 {
		config.MaxTokens = 100
	}
	if config.RefillRate <= 0 {
		config.RefillRate = 10
	}

	return &TokenBucketLimiter{
		tokens:     float64(config.MaxTokens),
		maxTokens:  float64(config.MaxTokens),
		refillRate: config.RefillRate,
		lastRefill: time.Now(),
	}
}

func (l *TokenBucketLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens += elapsed * l.refillRate
	if l.tokens > l.maxTokens {
		l.tokens = l.maxTokens
	}
	l.lastRefill = now
}

func (l *TokenBucketLimiter) Allow() bool {
	return l.AllowN(1)
}

func (l *TokenBucketLimiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}
	return false
}

func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
	return l.WaitN(ctx, 1)
}

func (l *TokenBucketLimiter) WaitN(ctx context.Context, n int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if l.AllowN(n) {
				return nil
			}
		}
	}
}

// SlidingWindowLimiter 滑动窗口限流器
type SlidingWindowLimiter struct {
	mu         sync.Mutex
	windowSize time.Duration // 窗口大小
	maxReqs    int           // 窗口内最大请求数
	timestamps []time.Time   // 请求时间戳
}

// SlidingWindowConfig 滑动窗口配置
type SlidingWindowConfig struct {
	WindowSize time.Duration // 窗口大小
	MaxReqs    int           // 窗口内最大请求数
}

// NewSlidingWindowLimiter 创建滑动窗口限流器
func NewSlidingWindowLimiter(config SlidingWindowConfig) *SlidingWindowLimiter {
	if config.WindowSize <= 0 {
		config.WindowSize = time.Second
	}
	if config.MaxReqs <= 0 {
		config.MaxReqs = 100
	}

	return &SlidingWindowLimiter{
		windowSize: config.WindowSize,
		maxReqs:    config.MaxReqs,
		timestamps: make([]time.Time, 0),
	}
}

func (l *SlidingWindowLimiter) cleanup() {
	threshold := time.Now().Add(-l.windowSize)
	i := 0
	for ; i < len(l.timestamps); i++ {
		if l.timestamps[i].After(threshold) {
			break
		}
	}
	l.timestamps = l.timestamps[i:]
}

func (l *SlidingWindowLimiter) Allow() bool {
	return l.AllowN(1)
}

func (l *SlidingWindowLimiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanup()

	if len(l.timestamps)+n <= l.maxReqs {
		now := time.Now()
		for i := 0; i < n; i++ {
			l.timestamps = append(l.timestamps, now)
		}
		return true
	}
	return false
}

func (l *SlidingWindowLimiter) Wait(ctx context.Context) error {
	return l.WaitN(ctx, 1)
}

func (l *SlidingWindowLimiter) WaitN(ctx context.Context, n int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if l.AllowN(n) {
				return nil
			}
		}
	}
}

// ErrRateLimited 限流错误
var ErrRateLimited = gerr.NewGErr(429, "rate limited")

// RateLimitMiddleware 限流中间件配置
type RateLimitMiddleware struct {
	limiter  RateLimiter
	blocking bool // 是否阻塞等待
}

// NewRateLimitMiddleware 创建限流中间件
func NewRateLimitMiddleware(limiter RateLimiter, blocking bool) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter:  limiter,
		blocking: blocking,
	}
}

// Check 检查请求是否被限流
func (m *RateLimitMiddleware) Check(ctx context.Context) error {
	if m.blocking {
		return m.limiter.Wait(ctx)
	}
	if !m.limiter.Allow() {
		return ErrRateLimited
	}
	return nil
}
