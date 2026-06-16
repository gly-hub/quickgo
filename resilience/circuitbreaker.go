package resilience

import (
	"context"
	"sync"
	"time"

	"github.com/team-dandelion/quickgo/gerr"
)

// CircuitState 熔断器状态
type CircuitState int

const (
	// StateClosed 关闭状态（正常工作）
	StateClosed CircuitState = iota
	// StateOpen 打开状态（熔断）
	StateOpen
	// StateHalfOpen 半开状态（尝试恢复）
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	mu sync.RWMutex

	name          string        // 熔断器名称
	state         CircuitState  // 当前状态
	failureCount  int           // 连续失败次数
	successCount  int           // 半开状态下连续成功次数
	halfOpenReqs  int           // 半开状态下正在执行的探测请求数
	lastFailure   time.Time     // 最后失败时间
	lastStateTime time.Time     // 最后状态变化时间
	config        CircuitConfig // 配置
}

// CircuitConfig 熔断器配置
type CircuitConfig struct {
	// 失败阈值：连续失败多少次后熔断
	FailureThreshold int
	// 成功阈值：半开状态下连续成功多少次后关闭熔断
	SuccessThreshold int
	// 熔断持续时间：熔断多长时间后进入半开状态
	OpenDuration time.Duration
	// 半开状态最大并发请求数
	HalfOpenMaxReqs int
	// 失败判断函数（可选）
	IsFailure func(err error) bool
}

// DefaultCircuitConfig 默认配置
func DefaultCircuitConfig() CircuitConfig {
	return CircuitConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		OpenDuration:     30 * time.Second,
		HalfOpenMaxReqs:  1,
		IsFailure:        nil,
	}
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(name string, config CircuitConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 3
	}
	if config.OpenDuration <= 0 {
		config.OpenDuration = 30 * time.Second
	}
	if config.HalfOpenMaxReqs <= 0 {
		config.HalfOpenMaxReqs = 1
	}

	return &CircuitBreaker{
		name:          name,
		state:         StateClosed,
		config:        config,
		lastStateTime: time.Now(),
	}
}

// ErrCircuitOpen 熔断器打开错误
var ErrCircuitOpen = gerr.NewGErr(503, "circuit breaker is open")

// Allow 检查是否允许请求通过
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		// 检查是否可以进入半开状态
		if time.Since(cb.lastStateTime) >= cb.config.OpenDuration {
			cb.toHalfOpen()
			cb.halfOpenReqs++
			return nil
		}
		return ErrCircuitOpen
	case StateHalfOpen:
		if cb.halfOpenReqs >= cb.config.HalfOpenMaxReqs {
			return ErrCircuitOpen
		}
		cb.halfOpenReqs++
		return nil
	}

	return nil
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		cb.finishHalfOpenRequest()
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.toClosed()
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailure = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.toOpen()
		}
	case StateHalfOpen:
		cb.finishHalfOpenRequest()
		cb.toOpen()
	}
}

// Execute 执行函数，自动处理熔断逻辑
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn(ctx)
	if err != nil {
		if cb.isFailure(err) {
			cb.RecordFailure()
		}
		return err
	}

	cb.RecordSuccess()
	return nil
}

func (cb *CircuitBreaker) isFailure(err error) bool {
	if cb.config.IsFailure != nil {
		return cb.config.IsFailure(err)
	}
	return err != nil
}

func (cb *CircuitBreaker) toOpen() {
	cb.state = StateOpen
	cb.lastStateTime = time.Now()
	cb.successCount = 0
	cb.halfOpenReqs = 0
}

func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = StateHalfOpen
	cb.lastStateTime = time.Now()
	cb.successCount = 0
	cb.failureCount = 0
	cb.halfOpenReqs = 0
}

func (cb *CircuitBreaker) toClosed() {
	cb.state = StateClosed
	cb.lastStateTime = time.Now()
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenReqs = 0
}

func (cb *CircuitBreaker) finishHalfOpenRequest() {
	if cb.halfOpenReqs > 0 {
		cb.halfOpenReqs--
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Name 获取名称
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Stats 熔断器统计信息
type Stats struct {
	Name         string        `json:"name"`
	State        string        `json:"state"`
	FailureCount int           `json:"failureCount"`
	SuccessCount int           `json:"successCount"`
	HalfOpenReqs int           `json:"halfOpenReqs"`
	LastFailure  time.Time     `json:"lastFailure,omitempty"`
	Config       CircuitConfig `json:"-"`
}

// Stats 获取统计信息
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return Stats{
		Name:         cb.name,
		State:        cb.state.String(),
		FailureCount: cb.failureCount,
		SuccessCount: cb.successCount,
		HalfOpenReqs: cb.halfOpenReqs,
		LastFailure:  cb.lastFailure,
		Config:       cb.config,
	}
}

// CircuitBreakerManager 熔断器管理器
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitConfig
}

// NewCircuitBreakerManager 创建熔断器管理器
func NewCircuitBreakerManager(config CircuitConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// Get 获取或创建熔断器
func (m *CircuitBreakerManager) Get(name string) *CircuitBreaker {
	m.mu.RLock()
	if cb, ok := m.breakers[name]; ok {
		m.mu.RUnlock()
		return cb
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if cb, ok := m.breakers[name]; ok {
		return cb
	}

	cb := NewCircuitBreaker(name, m.config)
	m.breakers[name] = cb
	return cb
}

// AllStats 获取所有熔断器统计信息
func (m *CircuitBreakerManager) AllStats() []Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]Stats, 0, len(m.breakers))
	for _, cb := range m.breakers {
		stats = append(stats, cb.Stats())
	}
	return stats
}
