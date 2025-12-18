package quickgo

import (
	"context"
	"errors"
	"fmt"
	"github.com/team-dandelion/quickgo/http"
	"github.com/team-dandelion/quickgo/logger"

	"github.com/gofiber/fiber/v2"
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
	// CORS 配置
	CORS CORSConfig `json:"cors" yaml:"cors"`
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
	server *http.Server
	config *HTTPServerConfig
}

// NewHTTPServer 创建 HTTP 服务器实例
func NewHTTPServer(config *HTTPServerConfig) (*HTTPServer, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}

	// 设置默认值
	if config.Address == "" {
		config.Address = "0.0.0.0"
	}
	if config.Port == 0 {
		config.Port = 8080
	}

	// 构建 HTTP 服务器配置
	httpConfig := http.Config{
		Address:        config.Address,
		Port:           config.Port,
		EnableCORS:     config.EnableCORS,
		EnableRecovery: config.EnableRecovery,
		EnableLogging:  config.EnableLogging,
		EnableTrace:    config.EnableTrace,
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

	return &HTTPServer{
		server: server,
		config: config,
	}, nil
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

	go func() {
		ctx := context.Background()
		logger.Info(ctx, "HTTP server starting on %s:%d", s.config.Address, s.config.Port)
		if err := s.server.Start(); err != nil {
			logger.Fatal(ctx, "HTTP server failed to start: %v", err)
		}
	}()

	return nil
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
