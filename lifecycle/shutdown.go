// Package lifecycle 提供应用生命周期管理功能
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/team-dandelion/quickgo/logger"
)

// ShutdownFunc 关闭函数类型
type ShutdownFunc func(ctx context.Context) error

// ShutdownHook 关闭钩子
type ShutdownHook struct {
	Name     string        // 钩子名称
	Priority int           // 优先级（数字越小越先执行）
	Func     ShutdownFunc  // 关闭函数
	Timeout  time.Duration // 单独的超时时间（0表示使用全局超时）
}

// ShutdownManager 优雅关闭管理器
type ShutdownManager struct {
	mu              sync.Mutex
	hooks           []ShutdownHook
	globalTimeout   time.Duration
	signals         []os.Signal
	shutdownOnce    sync.Once
	shutdownErr     error
	shutdownStarted bool
	waitStarted     bool
	waitAsyncOnce   sync.Once
	done            chan struct{}
}

// ShutdownConfig 关闭管理器配置
type ShutdownConfig struct {
	// GlobalTimeout 全局超时时间
	GlobalTimeout time.Duration
	// Signals 监听的信号
	Signals []os.Signal
}

// DefaultShutdownConfig 默认配置
func DefaultShutdownConfig() ShutdownConfig {
	return ShutdownConfig{
		GlobalTimeout: 30 * time.Second,
		Signals:       []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT},
	}
}

// NewShutdownManager 创建关闭管理器
func NewShutdownManager(config ShutdownConfig) *ShutdownManager {
	if config.GlobalTimeout <= 0 {
		config.GlobalTimeout = 30 * time.Second
	}
	if len(config.Signals) == 0 {
		config.Signals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT}
	}

	return &ShutdownManager{
		hooks:         make([]ShutdownHook, 0),
		globalTimeout: config.GlobalTimeout,
		signals:       config.Signals,
		done:          make(chan struct{}),
	}
}

// 全局管理器实例
var (
	globalManager     *ShutdownManager
	globalManagerOnce sync.Once
)

// Global 获取全局管理器实例
func Global() *ShutdownManager {
	globalManagerOnce.Do(func() {
		globalManager = NewShutdownManager(DefaultShutdownConfig())
	})
	return globalManager
}

// Register 注册关闭钩子
func (m *ShutdownManager) Register(hook ShutdownHook) {
	m.mu.Lock()
	if m.shutdownStarted {
		m.mu.Unlock()
		logger.Warn(context.Background(), "Ignoring shutdown hook registered after shutdown started: %s", hook.Name)
		return
	}

	m.hooks = append(m.hooks, hook)
	m.mu.Unlock()
}

// RegisterFunc 注册简单的关闭函数
func (m *ShutdownManager) RegisterFunc(name string, priority int, fn ShutdownFunc) {
	m.Register(ShutdownHook{
		Name:     name,
		Priority: priority,
		Func:     fn,
	})
}

// RegisterFuncWithTimeout 注册带超时的关闭函数
func (m *ShutdownManager) RegisterFuncWithTimeout(name string, priority int, timeout time.Duration, fn ShutdownFunc) {
	m.Register(ShutdownHook{
		Name:     name,
		Priority: priority,
		Func:     fn,
		Timeout:  timeout,
	})
}

// Shutdown 执行关闭
func (m *ShutdownManager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.shutdownStarted = true
	m.mu.Unlock()

	m.shutdownOnce.Do(func() {
		defer close(m.done)

		m.mu.Lock()
		hooks := make([]ShutdownHook, len(m.hooks))
		copy(hooks, m.hooks)
		globalTimeout := m.globalTimeout
		m.mu.Unlock()
		shutdownCtx, cancel := context.WithTimeout(ctx, globalTimeout)
		defer cancel()

		// 按优先级排序
		sort.Slice(hooks, func(i, j int) bool {
			return hooks[i].Priority < hooks[j].Priority
		})

		logger.Info(shutdownCtx, "Starting graceful shutdown, %d hooks to execute", len(hooks))

		var shutdownErrs []error
		for _, hook := range hooks {
			timeout := hook.Timeout
			if timeout <= 0 {
				timeout = globalTimeout
			}

			hookCtx, cancel := context.WithTimeout(shutdownCtx, timeout)

			logger.Info(shutdownCtx, "Executing shutdown hook: %s (priority: %d)", hook.Name, hook.Priority)

			start := time.Now()
			err := runShutdownHook(hookCtx, hook)
			duration := time.Since(start)

			cancel()

			if err != nil {
				logger.Error(shutdownCtx, "Shutdown hook '%s' failed after %v: %v", hook.Name, duration, err)
				shutdownErrs = append(shutdownErrs, fmt.Errorf("shutdown hook %s: %w", hook.Name, err))
			} else {
				logger.Info(shutdownCtx, "Shutdown hook '%s' completed in %v", hook.Name, duration)
			}
			if shutdownCtx.Err() != nil {
				break
			}
		}

		m.mu.Lock()
		m.shutdownErr = errors.Join(shutdownErrs...)
		m.mu.Unlock()
		logger.Info(shutdownCtx, "Graceful shutdown completed")
	})

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownErr
}

func runShutdownHook(ctx context.Context, hook ShutdownHook) error {
	if hook.Func == nil {
		return errors.New("shutdown hook function is nil")
	}

	result := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				result <- fmt.Errorf("panic: %v", recovered)
			}
		}()
		result <- hook.Func(ctx)
	}()

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait 等待信号并执行关闭
func (m *ShutdownManager) Wait() {
	m.mu.Lock()
	if m.waitStarted {
		m.mu.Unlock()
		<-m.done
		return
	}
	m.waitStarted = true
	signals := append([]os.Signal(nil), m.signals...)
	globalTimeout := m.globalTimeout
	m.mu.Unlock()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)
	defer signal.Stop(sigChan)

	select {
	case sig := <-sigChan:
		logger.Info(context.Background(), "Received signal: %v, initiating graceful shutdown", sig)

		ctx, cancel := context.WithTimeout(context.Background(), globalTimeout)
		defer cancel()

		if err := m.Shutdown(ctx); err != nil {
			logger.Error(ctx, "Graceful shutdown completed with errors: %v", err)
		}
	case <-m.done:
	}
}

// WaitAsync 异步等待信号
func (m *ShutdownManager) WaitAsync() <-chan struct{} {
	m.waitAsyncOnce.Do(func() {
		go m.Wait()
	})
	return m.done
}

// Done 返回关闭完成的通道
func (m *ShutdownManager) Done() <-chan struct{} {
	return m.done
}

// SetGlobalTimeout 设置全局超时时间
func (m *ShutdownManager) SetGlobalTimeout(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalTimeout = timeout
}

// 便捷的全局函数

// Register 注册关闭钩子到全局管理器
func Register(hook ShutdownHook) {
	Global().Register(hook)
}

// RegisterFunc 注册简单的关闭函数到全局管理器
func RegisterFunc(name string, priority int, fn ShutdownFunc) {
	Global().RegisterFunc(name, priority, fn)
}

// Shutdown 执行全局关闭
func Shutdown(ctx context.Context) error {
	return Global().Shutdown(ctx)
}

// Wait 等待信号并执行全局关闭
func Wait() {
	Global().Wait()
}

// 常用优先级常量
const (
	PriorityFirst    = 0    // 最先执行
	PriorityHTTP     = 100  // HTTP 服务器
	PriorityGRPC     = 200  // gRPC 服务器
	PriorityWorker   = 300  // 工作协程
	PriorityQueue    = 400  // 消息队列
	PriorityCache    = 500  // 缓存
	PriorityDatabase = 600  // 数据库
	PriorityTracing  = 700  // 链路追踪
	PriorityLogger   = 800  // 日志
	PriorityLast     = 1000 // 最后执行
)

// DrainConfig 排空配置
type DrainConfig struct {
	// CheckInterval 检查间隔
	CheckInterval time.Duration
	// Timeout 超时时间
	Timeout time.Duration
}

// DrainFunc 排空检查函数，返回 true 表示已排空
type DrainFunc func() bool

// Drain 等待直到排空或超时
func Drain(ctx context.Context, config DrainConfig, check DrainFunc) error {
	if config.CheckInterval <= 0 {
		config.CheckInterval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if check() {
				return nil
			}
		}
	}
}
