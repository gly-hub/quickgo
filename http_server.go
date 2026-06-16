package quickgo

import (
	"context"
	"errors"
	"fmt"

	"github.com/team-dandelion/quickgo/http"
	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/metrics"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

type AppRouteHandler func(app *fiber.App)

// HTTPServerConfig HTTP 服务器配置
type HTTPServerConfig struct {
	// 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	// 监听地址
	Address string `json:"address" yaml:"address"`
	// 监听端口
	Port int `json:"port" yaml:"port"`
	// 是否启用 CORS
	EnableCORS bool `json:"enableCORS" yaml:"enableCORS"`
	// 是否启用恢复中间件
	EnableRecovery bool `json:"enableRecovery" yaml:"enableRecovery"`
	// 是否启用日志中间件
	EnableLogging bool `json:"enableLogging" yaml:"enableLogging"`
	// 是否启用链路追踪中间件
	EnableTrace bool `json:"enableTrace" yaml:"enableTrace"`
	// 显式禁用 CORS
	DisableCORS bool `json:"disableCORS" yaml:"disableCORS"`
	// 显式禁用恢复中间件
	DisableRecovery bool `json:"disableRecovery" yaml:"disableRecovery"`
	// 显式禁用日志中间件
	DisableLogging bool `json:"disableLogging" yaml:"disableLogging"`
	// 显式禁用链路追踪中间件
	DisableTrace bool `json:"disableTrace" yaml:"disableTrace"`
	// CORS 配置
	CORS CORSConfig `json:"cors" yaml:"cors"`
	// Metrics 配置（可选）
	Metrics *metrics.Config `json:"metrics" yaml:"metrics"`
	// MetricsPath 指标暴露路径，默认 /metrics
	MetricsPath string `json:"metricsPath" yaml:"metricsPath"`
	// DisableMetricsEndpoint 显式禁用 /metrics 路由
	DisableMetricsEndpoint bool `json:"disableMetricsEndpoint" yaml:"disableMetricsEndpoint"`

	metrics *metrics.Metrics
}

// CORSConfig CORS 配置
type CORSConfig struct {
	AllowOrigins     string `json:"allowOrigins" yaml:"allowOrigins"`         // 允许的源
	AllowMethods     string `json:"allowMethods" yaml:"allowMethods"`         // 允许的方法
	AllowHeaders     string `json:"allowHeaders" yaml:"allowHeaders"`         // 允许的请求头
	AllowCredentials bool   `json:"allowCredentials" yaml:"allowCredentials"` // 是否允许凭证
	ExposeHeaders    string `json:"exposeHeaders" yaml:"exposeHeaders"`       // 暴露的响应头
	MaxAge           int    `json:"maxAge" yaml:"maxAge"`                     // 预检请求缓存时间（秒）
}

// HTTPServer HTTP 服务器封装
type HTTPServer struct {
	server  *http.Server
	config  *HTTPServerConfig
	metrics *metrics.Metrics
}

// NewHTTPServer 创建 HTTP 服务器实例
func NewHTTPServer(config *HTTPServerConfig) (*HTTPServer, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}
	config = cloneHTTPServerConfig(config)

	// 设置默认值
	if config.Address == "" {
		config.Address = "0.0.0.0"
	}
	if config.Port == 0 {
		config.Port = 8080
	}

	// 构建 HTTP 服务器配置
	httpConfig := http.Config{
		Address:         config.Address,
		Port:            config.Port,
		EnableCORS:      config.EnableCORS,
		EnableRecovery:  config.EnableRecovery,
		EnableLogging:   config.EnableLogging,
		EnableTrace:     config.EnableTrace,
		DisableCORS:     config.DisableCORS,
		DisableRecovery: config.DisableRecovery,
		DisableLogging:  config.DisableLogging,
		DisableTrace:    config.DisableTrace,
	}
	metricCollector := config.metrics
	if metricCollector == nil && config.Metrics != nil {
		metricCollector = metrics.New(*config.Metrics)
	}
	if metricCollector != nil {
		httpConfig.Middlewares = append(httpConfig.Middlewares, metrics.FiberMiddleware(metricCollector))
	}

	// 设置 CORS 配置
	if config.CORS.AllowOrigins != "" {
		httpConfig.CORSConfig.AllowOrigins = config.CORS.AllowOrigins
	}
	if config.CORS.AllowMethods != "" {
		httpConfig.CORSConfig.AllowMethods = config.CORS.AllowMethods
	}
	if config.CORS.AllowHeaders != "" {
		httpConfig.CORSConfig.AllowHeaders = config.CORS.AllowHeaders
	}
	httpConfig.CORSConfig.AllowCredentials = config.CORS.AllowCredentials
	if config.CORS.ExposeHeaders != "" {
		httpConfig.CORSConfig.ExposeHeaders = config.CORS.ExposeHeaders
	}
	if config.CORS.MaxAge > 0 {
		httpConfig.CORSConfig.MaxAge = config.CORS.MaxAge
	}

	// 创建 HTTP 服务器
	server, err := http.NewServer(httpConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create http server: %w", err)
	}

	if metricCollector != nil && !config.DisableMetricsEndpoint {
		metricsPath := config.MetricsPath
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		server.GetApp().Get(metricsPath, adaptor.HTTPHandler(metricCollector.Handler()))
	}

	return &HTTPServer{
		server:  server,
		config:  config,
		metrics: metricCollector,
	}, nil
}

func cloneHTTPServerConfig(config *HTTPServerConfig) *HTTPServerConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.Metrics != nil {
		metricsConfig := *config.Metrics
		if config.Metrics.Buckets != nil {
			metricsConfig.Buckets = append([]float64(nil), config.Metrics.Buckets...)
		}
		cloned.Metrics = &metricsConfig
	}
	return &cloned
}

// Start 启动 HTTP 服务器
func (s *HTTPServer) Start() error {
	if s.server == nil {
		return errors.New("server is nil")
	}

	ctx := context.Background()
	logger.Info(ctx, "HTTP server starting on %s:%d", s.config.Address, s.config.Port)
	return s.server.Start()
}

// StartAsync 异步启动 HTTP 服务器
func (s *HTTPServer) StartAsync() error {
	if s.server == nil {
		return errors.New("server is nil")
	}

	return s.server.StartAsync()
}

// Stop 停止 HTTP 服务器
func (s *HTTPServer) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx := context.Background()
	logger.Info(ctx, "HTTP server shutting down...")
	return s.server.Stop()
}

// GetApp 获取 Fiber 应用实例（用于注册路由等）
func (s *HTTPServer) GetApp() *fiber.App {
	return s.server.GetApp()
}

// GetServer 获取底层 HTTP 服务器实例
func (s *HTTPServer) GetServer() *http.Server {
	return s.server
}

// Metrics 获取 HTTP 服务器使用的指标收集器。
func (s *HTTPServer) Metrics() *metrics.Metrics {
	return s.metrics
}

func (s *HTTPServer) RegisterApp(handler AppRouteHandler) error {
	if s.server == nil {
		return errors.New("server is nil")
	}

	app := s.server.GetApp()
	handler(app)
	return nil
}

// RegisterRoute 注册路由（便捷方法）
// method: HTTP 方法（GET, POST, PUT, DELETE 等）
// path: 路由路径
// handler: 处理函数
func (s *HTTPServer) RegisterRoute(method, path string, handler fiber.Handler) error {
	if s.server == nil {
		return errors.New("server is nil")
	}

	app := s.server.GetApp()
	switch method {
	case "GET":
		app.Get(path, handler)
	case "POST":
		app.Post(path, handler)
	case "PUT":
		app.Put(path, handler)
	case "DELETE":
		app.Delete(path, handler)
	case "PATCH":
		app.Patch(path, handler)
	case "HEAD":
		app.Head(path, handler)
	case "OPTIONS":
		app.Options(path, handler)
	default:
		return fmt.Errorf("unsupported HTTP method: %s", method)
	}

	return nil
}
