package lifecycle

import (
	"context"
	"sync"
	"time"
)

// HealthStatus 健康状态
type HealthStatus int

const (
	// StatusUnknown 未知状态
	StatusUnknown HealthStatus = iota
	// StatusHealthy 健康
	StatusHealthy
	// StatusUnhealthy 不健康
	StatusUnhealthy
	// StatusDegraded 降级
	StatusDegraded
)

func (s HealthStatus) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusUnhealthy:
		return "unhealthy"
	case StatusDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// HealthCheck 健康检查接口
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) HealthResult
}

// HealthResult 健康检查结果
type HealthResult struct {
	Status  HealthStatus          `json:"status"`
	Message string                `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
	Time    time.Time             `json:"time"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	mu         sync.RWMutex
	checks     map[string]HealthCheck
	timeout    time.Duration
	lastResult map[string]HealthResult
}

// HealthCheckerConfig 健康检查器配置
type HealthCheckerConfig struct {
	Timeout time.Duration
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(config HealthCheckerConfig) *HealthChecker {
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}

	return &HealthChecker{
		checks:     make(map[string]HealthCheck),
		timeout:    config.Timeout,
		lastResult: make(map[string]HealthResult),
	}
}

// Register 注册健康检查
func (h *HealthChecker) Register(check HealthCheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[check.Name()] = check
}

// RegisterFunc 注册健康检查函数
func (h *HealthChecker) RegisterFunc(name string, fn func(ctx context.Context) HealthResult) {
	h.Register(&funcHealthCheck{name: name, fn: fn})
}

type funcHealthCheck struct {
	name string
	fn   func(ctx context.Context) HealthResult
}

func (f *funcHealthCheck) Name() string {
	return f.name
}

func (f *funcHealthCheck) Check(ctx context.Context) HealthResult {
	return f.fn(ctx)
}

// Check 执行所有健康检查
func (h *HealthChecker) Check(ctx context.Context) map[string]HealthResult {
	h.mu.RLock()
	checks := make(map[string]HealthCheck)
	for k, v := range h.checks {
		checks[k] = v
	}
	h.mu.RUnlock()

	results := make(map[string]HealthResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, h.timeout)
			defer cancel()

			result := check.Check(checkCtx)
			result.Time = time.Now()

			mu.Lock()
			results[name] = result
			mu.Unlock()
		}(name, check)
	}

	wg.Wait()

	// 更新缓存
	h.mu.Lock()
	for k, v := range results {
		h.lastResult[k] = v
	}
	h.mu.Unlock()

	return results
}

// CheckOne 执行单个健康检查
func (h *HealthChecker) CheckOne(ctx context.Context, name string) (HealthResult, bool) {
	h.mu.RLock()
	check, ok := h.checks[name]
	h.mu.RUnlock()

	if !ok {
		return HealthResult{}, false
	}

	checkCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	result := check.Check(checkCtx)
	result.Time = time.Now()

	// 更新缓存
	h.mu.Lock()
	h.lastResult[name] = result
	h.mu.Unlock()

	return result, true
}

// LastResult 获取上次检查结果
func (h *HealthChecker) LastResult() map[string]HealthResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]HealthResult)
	for k, v := range h.lastResult {
		results[k] = v
	}
	return results
}

// OverallStatus 获取总体状态
func (h *HealthChecker) OverallStatus(ctx context.Context) HealthStatus {
	results := h.Check(ctx)

	hasUnhealthy := false
	hasDegraded := false

	for _, result := range results {
		switch result.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

// Readiness 就绪检查结果
type Readiness struct {
	Ready   bool              `json:"ready"`
	Checks  map[string]HealthResult `json:"checks,omitempty"`
}

// IsReady 检查是否就绪
func (h *HealthChecker) IsReady(ctx context.Context) Readiness {
	results := h.Check(ctx)

	ready := true
	for _, result := range results {
		if result.Status == StatusUnhealthy {
			ready = false
			break
		}
	}

	return Readiness{
		Ready:  ready,
		Checks: results,
	}
}

// Liveness 存活检查结果
type Liveness struct {
	Alive bool `json:"alive"`
}

// IsAlive 检查是否存活（简单检查）
func (h *HealthChecker) IsAlive() Liveness {
	return Liveness{Alive: true}
}
