package resilience

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxAttempts 最大重试次数（包括首次尝试）
	MaxAttempts int
	// InitialDelay 初始延迟时间
	InitialDelay time.Duration
	// MaxDelay 最大延迟时间
	MaxDelay time.Duration
	// Multiplier 延迟倍数（指数退避）
	Multiplier float64
	// Jitter 抖动因子（0-1之间）
	Jitter float64
	// RetryIf 判断是否需要重试的函数
	RetryIf func(err error) bool
	// OnRetry 重试时的回调函数
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryIf:      nil,
		OnRetry:      nil,
	}
}

// Retryer 重试器
type Retryer struct {
	config RetryConfig
}

// NewRetryer 创建重试器
func NewRetryer(config RetryConfig) *Retryer {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 10 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	if config.Jitter < 0 || config.Jitter > 1 {
		config.Jitter = 0.1
	}

	return &Retryer{config: config}
}

// ErrMaxRetriesExceeded 超过最大重试次数错误
var ErrMaxRetriesExceeded = errors.New("max retries exceeded")

// RetryResult 重试结果
type RetryResult struct {
	Attempts int           // 尝试次数
	Duration time.Duration // 总耗时
	LastErr  error         // 最后一次错误
}

// Do 执行带重试的操作
func (r *Retryer) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	_, err := r.DoWithResult(ctx, fn)
	return err
}

// DoWithResult 执行带重试的操作并返回详细结果
func (r *Retryer) DoWithResult(ctx context.Context, fn func(ctx context.Context) error) (*RetryResult, error) {
	start := time.Now()
	result := &RetryResult{}

	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		// 执行操作
		err := fn(ctx)
		if err == nil {
			result.Duration = time.Since(start)
			return result, nil
		}

		lastErr = err
		result.LastErr = err

		// 检查是否需要重试
		if r.config.RetryIf != nil && !r.config.RetryIf(err) {
			result.Duration = time.Since(start)
			return result, err
		}

		// 如果是最后一次尝试，不需要等待
		if attempt == r.config.MaxAttempts {
			break
		}

		// 计算延迟时间
		actualDelay := r.calculateDelay(delay, attempt)

		// 回调
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt, err, actualDelay)
		}

		// 等待
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			return result, ctx.Err()
		case <-time.After(actualDelay):
		}

		// 更新下一次延迟
		delay = time.Duration(float64(delay) * r.config.Multiplier)
		if delay > r.config.MaxDelay {
			delay = r.config.MaxDelay
		}
	}

	result.Duration = time.Since(start)
	return result, lastErr
}

func (r *Retryer) calculateDelay(baseDelay time.Duration, attempt int) time.Duration {
	delay := baseDelay

	// 添加抖动
	if r.config.Jitter > 0 {
		jitter := float64(delay) * r.config.Jitter
		delay += time.Duration(rand.Float64() * jitter)
	}

	return delay
}

// Retry 便捷函数：使用默认配置执行重试
func Retry(ctx context.Context, fn func(ctx context.Context) error) error {
	return NewRetryer(DefaultRetryConfig()).Do(ctx, fn)
}

// RetryWithConfig 便捷函数：使用指定配置执行重试
func RetryWithConfig(ctx context.Context, config RetryConfig, fn func(ctx context.Context) error) error {
	return NewRetryer(config).Do(ctx, fn)
}

// RetryN 便捷函数：指定重试次数
func RetryN(ctx context.Context, maxAttempts int, fn func(ctx context.Context) error) error {
	config := DefaultRetryConfig()
	config.MaxAttempts = maxAttempts
	return NewRetryer(config).Do(ctx, fn)
}

// BackoffPolicy 退避策略
type BackoffPolicy int

const (
	// BackoffConstant 固定延迟
	BackoffConstant BackoffPolicy = iota
	// BackoffLinear 线性增长
	BackoffLinear
	// BackoffExponential 指数增长
	BackoffExponential
)

// BackoffConfig 退避配置
type BackoffConfig struct {
	Policy       BackoffPolicy
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64 // 仅用于指数退避
}

// Backoff 计算退避时间
func Backoff(config BackoffConfig, attempt int) time.Duration {
	var delay time.Duration

	switch config.Policy {
	case BackoffConstant:
		delay = config.InitialDelay
	case BackoffLinear:
		delay = config.InitialDelay * time.Duration(attempt)
	case BackoffExponential:
		multiplier := config.Multiplier
		if multiplier <= 0 {
			multiplier = 2.0
		}
		delay = time.Duration(float64(config.InitialDelay) * math.Pow(multiplier, float64(attempt-1)))
	}

	if delay > config.MaxDelay && config.MaxDelay > 0 {
		delay = config.MaxDelay
	}

	return delay
}

// IsRetryableError 检查错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否是我们定义的可重试错误
	var gErr interface{ IsRetryable() bool }
	if errors.As(err, &gErr) {
		return gErr.IsRetryable()
	}

	// 上下文错误不重试
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// 默认可重试
	return true
}
