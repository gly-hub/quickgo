package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/tracing"
)

// Server HTTP服务器封装
type Server struct {
	app      *fiber.App
	address  string
	port     int
	config   Config
	listener net.Listener
	mu       sync.RWMutex
	running  bool
	stopped  bool
}

// Config HTTP服务器配置
type Config struct {
	Address string // 监听地址，默认 "0.0.0.0"
	Port    int    // 监听端口，默认 8080
	// Fiber 配置
	FiberConfig fiber.Config // Fiber 应用配置
	// 中间件配置
	EnableCORS     bool       // 是否启用 CORS，默认 true
	CORSConfig     CORSConfig // CORS 配置
	EnableRecovery bool       // 是否启用恢复中间件，默认 true
	EnableLogging  bool       // 是否启用日志中间件，默认 true
	EnableTrace    bool       // 是否启用链路追踪中间件，默认 true
	// 自定义中间件
	Middlewares []fiber.Handler // 自定义中间件列表
}

// CORSConfig CORS 配置
type CORSConfig struct {
	AllowOrigins     string // 允许的源，默认 "*"
	AllowMethods     string // 允许的方法，默认 "GET,POST,HEAD,PUT,DELETE,PATCH"
	AllowHeaders     string // 允许的请求头，默认 "*"
	AllowCredentials bool   // 是否允许凭证，默认 false
	ExposeHeaders    string // 暴露的响应头
	MaxAge           int    // 预检请求缓存时间（秒），默认 0
}

// NewServer 创建新的 HTTP 服务器实例
func NewServer(config Config) (*Server, error) {
	// 设置默认值
	if config.Address == "" {
		config.Address = "0.0.0.0"
	}
	if config.Port == 0 {
		config.Port = 8080
	}
	config.applyMiddlewareDefaults()

	// 设置 Fiber 默认配置
	fiberCfg := config.FiberConfig
	if fiberCfg.ErrorHandler == nil {
		fiberCfg.ErrorHandler = defaultErrorHandler
	}
	if fiberCfg.ReadTimeout == 0 {
		fiberCfg.ReadTimeout = 10 * time.Second
	}
	if fiberCfg.WriteTimeout == 0 {
		fiberCfg.WriteTimeout = 10 * time.Second
	}

	// 创建 Fiber 应用
	app := fiber.New(fiberCfg)

	server := &Server{
		app:     app,
		address: config.Address,
		port:    config.Port,
		config:  config,
	}

	// 注册默认中间件
	server.registerDefaultMiddlewares()

	// 注册自定义中间件
	for _, middleware := range config.Middlewares {
		app.Use(middleware)
	}

	return server, nil
}

func (c *Config) applyMiddlewareDefaults() {
	if c.EnableCORS || c.EnableRecovery || c.EnableLogging || c.EnableTrace {
		return
	}
	c.EnableCORS = true
	c.EnableRecovery = true
	c.EnableLogging = true
	c.EnableTrace = true
}

// registerDefaultMiddlewares 注册默认中间件
func (s *Server) registerDefaultMiddlewares() {
	// 链路追踪中间件（应该最先执行，以便后续中间件可以使用 trace ID）
	if s.config.EnableTrace {
		// 如果 OpenTelemetry tracing 已启用，使用 OpenTelemetry 中间件
		// 否则使用自定义的 TraceMiddleware（用于日志关联）
		if tracing.IsEnabled() {
			s.app.Use(tracing.Middleware())
		} else {
			s.app.Use(TraceMiddleware())
		}
	}

	// 日志中间件
	if s.config.EnableLogging {
		s.app.Use(LoggingMiddleware())
	}

	// 恢复中间件
	if s.config.EnableRecovery {
		s.app.Use(recover.New(recover.Config{
			EnableStackTrace: true,
		}))
	}

	// CORS 中间件
	if s.config.EnableCORS {
		corsCfg := cors.Config{
			AllowOrigins:     s.config.CORSConfig.AllowOrigins,
			AllowMethods:     s.config.CORSConfig.AllowMethods,
			AllowHeaders:     s.config.CORSConfig.AllowHeaders,
			AllowCredentials: s.config.CORSConfig.AllowCredentials,
			ExposeHeaders:    s.config.CORSConfig.ExposeHeaders,
			MaxAge:           s.config.CORSConfig.MaxAge,
		}
		// 设置默认值
		if corsCfg.AllowOrigins == "" {
			corsCfg.AllowOrigins = "*"
		}
		if corsCfg.AllowMethods == "" {
			corsCfg.AllowMethods = "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS"
		}
		if corsCfg.AllowHeaders == "" {
			corsCfg.AllowHeaders = "*"
		}
		s.app.Use(cors.New(corsCfg))
	}
}

// GetApp 获取 Fiber 应用实例（用于注册路由等）
func (s *Server) GetApp() *fiber.App {
	return s.app
}

// Start 启动 HTTP 服务器
func (s *Server) Start() error {
	if s.isStopped() {
		return fmt.Errorf("http server already stopped")
	}
	if err := s.Listen(); err != nil {
		return err
	}
	if err := s.markRunning(); err != nil {
		return err
	}

	ctx := context.Background()
	logger.Info(ctx, "HTTP server starting on %s", s.GetAddress())
	listener := s.getListener()
	if err := s.app.Listener(listener); err != nil {
		s.clearRuntimeState()
		if isHTTPServerClosedError(err) {
			return nil
		}
		return err
	}
	s.clearRuntimeState()
	return nil
}

// Listen 绑定监听地址，但不启动 HTTP 服务。
func (s *Server) Listen() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return fmt.Errorf("http server already stopped")
	}
	if s.running {
		return fmt.Errorf("http server already running")
	}
	if s.listener != nil {
		return nil
	}

	listener, err := net.Listen("tcp", s.GetAddress())
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.GetAddress(), err)
	}
	s.listener = listener
	return nil
}

// StartAsync 异步启动 HTTP 服务器
func (s *Server) StartAsync() error {
	if s.isStopped() {
		return fmt.Errorf("http server already stopped")
	}
	if err := s.Listen(); err != nil {
		return err
	}
	if err := s.markRunning(); err != nil {
		return err
	}

	listener := s.getListener()
	go func() {
		ctx := context.Background()
		logger.Info(ctx, "HTTP server starting on %s", s.GetAddress())
		if err := s.app.Listener(listener); err != nil {
			if !isHTTPServerClosedError(err) {
				logger.Error(ctx, "HTTP server failed to start: %v", err)
			}
		}
		s.clearRuntimeState()
	}()
	return nil
}

// Stop 停止 HTTP 服务器
func (s *Server) Stop() error {
	ctx := context.Background()
	if s.isStopped() {
		return nil
	}
	logger.Info(ctx, "HTTP server shutting down...")
	err := errors.Join(s.app.Shutdown(), s.closeListener())
	s.setStopped()
	if isHTTPServerClosedError(err) {
		return nil
	}
	return err
}

// GetAddress 获取服务器地址
func (s *Server) GetAddress() string {
	return fmt.Sprintf("%s:%d", s.address, s.port)
}

// IsRunning 检查 HTTP 服务器是否正在运行。
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *Server) markRunning() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return fmt.Errorf("http server already stopped")
	}
	if s.running {
		return fmt.Errorf("http server already running")
	}
	if s.listener == nil {
		return fmt.Errorf("http server listener is not initialized")
	}
	s.running = true
	return nil
}

func (s *Server) getListener() net.Listener {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listener
}

func (s *Server) clearRuntimeState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.listener = nil
}

func (s *Server) closeListener() error {
	s.mu.Lock()
	listener := s.listener
	s.running = false
	s.listener = nil
	s.mu.Unlock()

	if listener == nil {
		return nil
	}
	if err := listener.Close(); err != nil && !isHTTPServerClosedError(err) {
		return err
	}
	return nil
}

func (s *Server) setStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
}

func (s *Server) isStopped() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopped
}

func isHTTPServerClosedError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed) ||
		strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "Server closed") ||
		strings.Contains(err.Error(), "server is not running")
}

// defaultErrorHandler 默认错误处理器
func defaultErrorHandler(c *fiber.Ctx, err error) error {
	// 从 Locals 中获取 trace ID，创建 context
	traceID := GetTraceID(c)
	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	// 记录错误日志
	logger.Error(ctx, "HTTP request error: %v", err)

	// 默认返回 500 错误
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
		"code":  code,
	})
}
