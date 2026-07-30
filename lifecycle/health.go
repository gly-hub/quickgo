package lifecycle

import (
	"context"
	"fmt"
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
	Status  HealthStatus           `json:"status"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
	Time    time.Time              `json:"time"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	mu         sync.RWMutex
	checks     map[string]HealthCheck
	timeout    time.Duration
	lastResult map[string]HealthResult
	slots      chan struct{}
}

// HealthCheckerConfig 健康检查器配置
type HealthCheckerConfig struct {
	Timeout       time.Duration
	MaxConcurrent int
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(config HealthCheckerConfig) *HealthChecker {
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 64
	}

	return &HealthChecker{
		checks:     make(map[string]HealthCheck),
		timeout:    config.Timeout,
		lastResult: make(map[string]HealthResult),
		slots:      make(chan struct{}, config.MaxConcurrent),
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

	type checkResult struct {
		name   string
		result HealthResult
	}

	results := make(map[string]HealthResult, len(checks))
	if len(checks) == 0 {
		return results
	}

	workerCount := len(checks)
	if maxConcurrent := cap(h.slots); workerCount > maxConcurrent {
		workerCount = maxConcurrent
	}

	type checkJob struct {
		name  string
		check HealthCheck
	}
	jobs := make(chan checkJob)
	resultCh := make(chan checkResult, len(checks))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for job := range jobs {
				resultCh <- checkResult{name: job.name, result: h.runCheck(ctx, job.name, job.check)}
			}
		}()
	}

	go func() {
		workers.Wait()
		close(resultCh)
	}()
	for name, check := range checks {
		jobs <- checkJob{name: name, check: check}
	}
	close(jobs)

	for result := range resultCh {
		results[result.name] = cloneHealthResult(result.result)
	}

	// 更新缓存
	h.mu.Lock()
	for k, v := range results {
		h.lastResult[k] = cloneHealthResult(v)
	}
	h.mu.Unlock()

	return cloneHealthResults(results)
}

func (h *HealthChecker) runCheck(ctx context.Context, name string, check HealthCheck) HealthResult {
	checkCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	resultCh := make(chan HealthResult, 1)
	select {
	case h.slots <- struct{}{}:
	case <-checkCtx.Done():
		return HealthResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("health check %s timed out waiting for an execution slot: %v", name, checkCtx.Err()),
			Time:    time.Now(),
		}
	}
	go func() {
		defer func() { <-h.slots }()
		defer func() {
			if r := recover(); r != nil {
				resultCh <- HealthResult{
					Status:  StatusUnhealthy,
					Message: fmt.Sprintf("health check panic: %v", r),
					Time:    time.Now(),
				}
			}
		}()

		result := check.Check(checkCtx)
		result.Time = time.Now()
		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		if result.Time.IsZero() {
			result.Time = time.Now()
		}
		return result
	case <-checkCtx.Done():
		return HealthResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("health check %s timed out: %v", name, checkCtx.Err()),
			Time:    time.Now(),
		}
	}
}

// CheckOne 执行单个健康检查
func (h *HealthChecker) CheckOne(ctx context.Context, name string) (HealthResult, bool) {
	h.mu.RLock()
	check, ok := h.checks[name]
	h.mu.RUnlock()

	if !ok {
		return HealthResult{}, false
	}

	result := cloneHealthResult(h.runCheck(ctx, name, check))

	// 更新缓存
	h.mu.Lock()
	h.lastResult[name] = cloneHealthResult(result)
	h.mu.Unlock()

	return result, true
}

// LastResult 获取上次检查结果
func (h *HealthChecker) LastResult() map[string]HealthResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]HealthResult)
	for k, v := range h.lastResult {
		results[k] = cloneHealthResult(v)
	}
	return results
}

func cloneHealthResults(results map[string]HealthResult) map[string]HealthResult {
	cloned := make(map[string]HealthResult, len(results))
	for name, result := range results {
		cloned[name] = cloneHealthResult(result)
	}
	return cloned
}

func cloneHealthResult(result HealthResult) HealthResult {
	cloned := result
	if result.Details == nil {
		return cloned
	}
	cloned.Details = make(map[string]interface{}, len(result.Details))
	for key, value := range result.Details {
		cloned.Details[key] = value
	}
	return cloned
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
	Ready  bool                    `json:"ready"`
	Checks map[string]HealthResult `json:"checks,omitempty"`
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
