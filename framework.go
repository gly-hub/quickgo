package quickgo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"gly-hub/go-dandelion/quickgo/db/gorm"
	"gly-hub/go-dandelion/quickgo/db/mongodb"
	"gly-hub/go-dandelion/quickgo/db/redis"
	"gly-hub/go-dandelion/quickgo/logger"
	"gly-hub/go-dandelion/quickgo/tracing"
)

// Framework 主体框架，统一管理所有组件
type Framework struct {
	// 配置
	config *FrameworkConfig

	// 核心组件
	logger *logger.Logger

	// 服务组件
	grpcServer    *GrpcServer
	grpcClientMgr *GrpcClientManager
	httpServer    *HTTPServer

	// 数据库组件
	gormManager    *gorm.Manager
	mongodbManager *mongodb.Manager
	redisManager   *redis.Manager

	// 组件注册表（用于扩展）
	components map[string]Component

	// 生命周期管理
	mu          sync.RWMutex
	initialized bool
	started     bool
	stopped     bool
}

// FrameworkConfig 框架配置（内部使用）
type FrameworkConfig struct {
	// 应用配置
	App AppConfig

	// Logger 配置
	Logger *LoggerConfig

	// gRPC Server 配置（可选）
	GrpcServer *GrpcServerConfig

	// gRPC Client 配置（可选，网关场景使用）
	GrpcClient *GrpcClientConfig

	// HTTP Server 配置（可选）
	HTTPServer *HTTPServerConfig

	// 数据库配置（可选）
	Gorm    *gorm.GormManagerConfig
	MongoDB *mongodb.MongoManagerConfig
	Redis   *redis.RedisManagerConfig

	// 链路追踪配置（可选）
	Tracing *tracing.Config
}

// FrameworkOption 框架配置选项
type FrameworkOption func(*FrameworkConfig)

// AppConfig 应用配置
type AppConfig struct {
	Name    string `json:"name" yaml:"name" toml:"name"`          // 应用名称
	Version string `json:"version" yaml:"version" toml:"version"` // 应用版本
	Env     string `json:"env" yaml:"env" toml:"env"`             // 环境：local, develop, release, production
}

// LoggerConfig Logger 配置
type LoggerConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled" toml:"enabled"` // 是否启用
	Level   string `json:"level" yaml:"level" toml:"level"`       // 日志级别：debug, info, warn, error
	Output  string `json:"output" yaml:"output" toml:"output"`    // 输出方式：console, file
	File    string `json:"file" yaml:"file" toml:"file"`          // 文件路径（output=file 时）
	Service string `json:"service" yaml:"service" toml:"service"` // 服务名称
	Version string `json:"version" yaml:"version" toml:"version"` // 服务版本
}

// Component 组件接口（用于扩展）
type Component interface {
	// Name 返回组件名称
	Name() string
	// Init 初始化组件
	Init(ctx context.Context) error
	// Start 启动组件
	Start(ctx context.Context) error
	// Stop 停止组件
	Stop(ctx context.Context) error
	// IsEnabled 是否启用
	IsEnabled() bool
}

// NewFramework 创建框架实例
// 使用 Option 模式，显式指定需要初始化的组件
func NewFramework(opts ...FrameworkOption) (*Framework, error) {
	config := &FrameworkConfig{
		App: AppConfig{
			Name:    "quickgo-app",
			Version: "1.0.0",
			Env:     GetEnv(),
		},
	}

	// 应用所有选项
	for _, opt := range opts {
		opt(config)
	}

	// Logger 是必需的，如果没有配置，使用默认值
	if config.Logger == nil {
		config.Logger = &LoggerConfig{
			Enabled: true,
			Level:   "info",
			Output:  "console",
			Service: config.App.Name,
			Version: config.App.Version,
		}
	}

	f := &Framework{
		config:     config,
		components: make(map[string]Component),
	}

	return f, nil
}

// ==================== 配置选项函数 ====================

// ConfigOptionWithApp 配置应用信息
func ConfigOptionWithApp(app AppConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.App = app
	}
}

// ConfigOptionWithLogger 配置 Logger
func ConfigOptionWithLogger(logger LoggerConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.Logger = &logger
	}
}

// ConfigOptionWithGrpcServer 配置 gRPC Server
func ConfigOptionWithGrpcServer(server *GrpcServerConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.GrpcServer = server
	}
}

// ConfigOptionWithGrpcClient 配置 gRPC Client
func ConfigOptionWithGrpcClient(client *GrpcClientConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.GrpcClient = client
	}
}

// ConfigOptionWithHTTPServer 配置 HTTP Server
func ConfigOptionWithHTTPServer(server *HTTPServerConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.HTTPServer = server
	}
}

// ConfigOptionWithGorm 配置 GORM 数据库管理器
func ConfigOptionWithGorm(config *gorm.GormManagerConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.Gorm = config
	}
}

// ConfigOptionWithMongoDB 配置 MongoDB 数据库管理器
func ConfigOptionWithMongoDB(config *mongodb.MongoManagerConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.MongoDB = config
	}
}

// ConfigOptionWithRedis 配置 Redis 数据库管理器
func ConfigOptionWithRedis(config *redis.RedisManagerConfig) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.Redis = config
	}
}

// ConfigOptionWithTracing 配置链路追踪
func ConfigOptionWithTracing(config *tracing.Config) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.Tracing = config
	}
}

// Init 初始化所有组件
// 只初始化通过 Option 显式配置的组件
func (f *Framework) Init() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.initialized {
		return errors.New("framework already initialized")
	}

	ctx := context.Background()

	// 1. 初始化链路追踪（最优先，其他组件可能需要追踪）
	if f.config.Tracing != nil {
		if err := f.initTracing(ctx); err != nil {
			return fmt.Errorf("failed to init tracing: %w", err)
		}
	}

	// 2. 初始化 Logger（优先，其他组件需要日志）
	if f.config.Logger != nil && f.config.Logger.Enabled {
		if err := f.initLogger(ctx); err != nil {
			return fmt.Errorf("failed to init logger: %w", err)
		}
	} else {
		// 即使未启用，也创建一个默认 logger
		logger.Init(logger.Config{
			Level: logger.LevelInfo,
		})
		f.logger = logger.GetDefault()
	}

	// 3. 初始化 gRPC Server（仅当通过 Option 配置时）
	if f.config.GrpcServer != nil {
		if err := f.initGrpcServer(ctx); err != nil {
			return fmt.Errorf("failed to init grpc server: %w", err)
		}
	}

	// 4. 初始化 gRPC Client Manager（仅当通过 Option 配置时）
	if f.config.GrpcClient != nil {
		if err := f.initGrpcClientManager(ctx); err != nil {
			return fmt.Errorf("failed to init grpc client manager: %w", err)
		}
	}

	// 5. 初始化 HTTP Server（仅当通过 Option 配置时）
	if f.config.HTTPServer != nil && f.config.HTTPServer.Enabled {
		if err := f.initHTTPServer(ctx); err != nil {
			return fmt.Errorf("failed to init http server: %w", err)
		}
	}

	// 6. 初始化 GORM 数据库管理器（仅当通过 Option 配置时）
	if f.config.Gorm != nil {
		if err := f.initGormManager(ctx); err != nil {
			return fmt.Errorf("failed to init gorm manager: %w", err)
		}
	}

	// 7. 初始化 MongoDB 数据库管理器（仅当通过 Option 配置时）
	if f.config.MongoDB != nil {
		if err := f.initMongoDBManager(ctx); err != nil {
			return fmt.Errorf("failed to init mongodb manager: %w", err)
		}
	}

	// 8. 初始化 Redis 数据库管理器（仅当通过 Option 配置时）
	if f.config.Redis != nil {
		if err := f.initRedisManager(ctx); err != nil {
			return fmt.Errorf("failed to init redis manager: %w", err)
		}
	}

	// 9. 初始化自定义组件
	for _, component := range f.components {
		if component.IsEnabled() {
			if err := component.Init(ctx); err != nil {
				return fmt.Errorf("failed to init component %s: %w", component.Name(), err)
			}
		}
	}

	f.initialized = true
	logger.Info(ctx, "Framework initialized successfully")
	return nil
}

// Start 启动所有组件
func (f *Framework) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.initialized {
		return errors.New("framework not initialized, call Init() first")
	}

	if f.started {
		return errors.New("framework already started")
	}

	ctx := context.Background()

	// 1. 启动 gRPC Server
	if f.grpcServer != nil {
		if err := f.grpcServer.Start(); err != nil {
			return fmt.Errorf("failed to start grpc server: %w", err)
		}
		logger.Info(ctx, "gRPC server started")
	}

	// 2. 启动 HTTP Server
	if f.httpServer != nil {
		if err := f.httpServer.StartAsync(); err != nil {
			return fmt.Errorf("failed to start http server: %w", err)
		}
		logger.Info(ctx, "HTTP server started")
	}

	// 3. 启动自定义组件
	for _, component := range f.components {
		if component.IsEnabled() {
			if err := component.Start(ctx); err != nil {
				return fmt.Errorf("failed to start component %s: %w", component.Name(), err)
			}
		}
	}

	f.started = true
	logger.Info(ctx, "Framework started successfully")
	return nil
}

// Stop 停止所有组件
func (f *Framework) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.started {
		return nil // 未启动，无需停止
	}

	if f.stopped {
		return nil // 已停止
	}

	ctx := context.Background()

	// 按相反顺序停止组件

	// 1. 停止自定义组件
	for _, component := range f.components {
		if component.IsEnabled() {
			if err := component.Stop(ctx); err != nil {
				logger.Error(ctx, "Failed to stop component %s: %v", component.Name(), err)
			}
		}
	}

	// 2. 停止 HTTP Server
	if f.httpServer != nil {
		if err := f.httpServer.Stop(); err != nil {
			logger.Error(ctx, "Failed to stop http server: %v", err)
		}
	}

	// 3. 停止 gRPC Server
	if f.grpcServer != nil {
		if err := f.grpcServer.Stop(); err != nil {
			logger.Error(ctx, "Failed to stop grpc server: %v", err)
		}
	}

	// 4. 关闭 gRPC Client Manager
	if f.grpcClientMgr != nil {
		if err := f.grpcClientMgr.CloseAll(); err != nil {
			logger.Error(ctx, "Failed to close grpc client manager: %v", err)
		}
	}

	// 5. 关闭数据库连接
	if f.redisManager != nil {
		if err := f.redisManager.Close(); err != nil {
			logger.Error(ctx, "Failed to close redis manager: %v", err)
		}
	}

	if f.mongodbManager != nil {
		if err := f.mongodbManager.Close(); err != nil {
			logger.Error(ctx, "Failed to close mongodb manager: %v", err)
		}
	}

	if f.gormManager != nil {
		if err := f.gormManager.Close(); err != nil {
			logger.Error(ctx, "Failed to close gorm manager: %v", err)
		}
	}

	f.stopped = true
	// 关闭链路追踪
	if f.config.Tracing != nil && f.config.Tracing.Enabled {
		if err := tracing.Shutdown(ctx); err != nil {
			logger.Error(ctx, "Failed to shutdown tracing: %v", err)
		} else {
			logger.Info(ctx, "Tracing shutdown successfully")
		}
	}

	logger.Info(ctx, "Framework stopped")
	return nil
}

// Wait 等待中断信号（优雅关闭）
func (f *Framework) Wait() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info(context.Background(), "Received shutdown signal, stopping framework...")
	if err := f.Stop(); err != nil {
		logger.Error(context.Background(), "Error stopping framework: %v", err)
	}
}

// RegisterComponent 注册自定义组件
func (f *Framework) RegisterComponent(component Component) error {
	if component == nil {
		return errors.New("component is nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	name := component.Name()
	if name == "" {
		return errors.New("component name is empty")
	}

	if _, exists := f.components[name]; exists {
		return fmt.Errorf("component %s already registered", name)
	}

	f.components[name] = component
	logger.Info(context.Background(), "Component registered: %s", name)
	return nil
}

// GetComponent 获取自定义组件
func (f *Framework) GetComponent(name string) (Component, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	component, exists := f.components[name]
	if !exists {
		return nil, fmt.Errorf("component %s not found", name)
	}

	return component, nil
}

// ==================== 组件访问方法 ====================

// Logger 获取 Logger 实例
func (f *Framework) Logger() *logger.Logger {
	return f.logger
}

// GrpcServer 获取 gRPC 服务器实例
func (f *Framework) GrpcServer() *GrpcServer {
	return f.grpcServer
}

// GrpcClientManager 获取 gRPC 客户端管理器实例
func (f *Framework) GrpcClientManager() *GrpcClientManager {
	return f.grpcClientMgr
}

// HTTPServer 获取 HTTP 服务器实例
func (f *Framework) HTTPServer() *HTTPServer {
	return f.httpServer
}

// GormManager 获取 GORM 数据库管理器实例
func (f *Framework) GormManager() *gorm.Manager {
	return f.gormManager
}

// MongoManager 获取 MongoDB 数据库管理器实例
func (f *Framework) MongoManager() *mongodb.Manager {
	return f.mongodbManager
}

// RedisManager 获取 Redis 数据库管理器实例
func (f *Framework) RedisManager() *redis.Manager {
	return f.redisManager
}

// ==================== 内部初始化方法 ====================

// initLogger 初始化 Logger
func (f *Framework) initLogger(ctx context.Context) error {
	cfg := f.config.Logger

	// 解析日志级别
	var level logger.Level
	switch cfg.Level {
	case "debug":
		level = logger.LevelDebug
	case "info":
		level = logger.LevelInfo
	case "warn":
		level = logger.LevelWarn
	case "error":
		level = logger.LevelError
	default:
		level = logger.LevelInfo
	}

	// 构建 logger 配置
	loggerConfig := logger.Config{
		Level:   level,
		Service: cfg.Service,
		Version: cfg.Version,
	}

	// 设置输出方式
	if cfg.Output == "file" && cfg.File != "" {
		// 文件输出需要单独配置，这里先使用控制台输出
		// TODO: 支持文件输出配置
	}

	if err := logger.Init(loggerConfig); err != nil {
		return err
	}

	f.logger = logger.GetDefault()
	return nil
}

// initGrpcServer 初始化 gRPC 服务器
func (f *Framework) initGrpcServer(ctx context.Context) error {
	server, err := NewGrpcServer(f.config.GrpcServer)
	if err != nil {
		return err
	}

	f.grpcServer = server
	return nil
}

// initGrpcClientManager 初始化 gRPC 客户端管理器
func (f *Framework) initGrpcClientManager(ctx context.Context) error {
	manager, err := NewGrpcClientManager(f.config.GrpcClient)
	if err != nil {
		return err
	}

	f.grpcClientMgr = manager
	return nil
}

// initHTTPServer 初始化 HTTP 服务器
func (f *Framework) initHTTPServer(ctx context.Context) error {
	server, err := NewHTTPServer(f.config.HTTPServer)
	if err != nil {
		return err
	}

	f.httpServer = server
	return nil
}

// initGormManager 初始化 GORM 数据库管理器
func (f *Framework) initGormManager(ctx context.Context) error {
	manager, err := gorm.NewManager(f.config.Gorm)
	if err != nil {
		return err
	}
	f.gormManager = manager
	logger.Info(ctx, "GORM manager initialized")
	return nil
}

// initMongoDBManager 初始化 MongoDB 数据库管理器
func (f *Framework) initMongoDBManager(ctx context.Context) error {
	manager, err := mongodb.NewManager(f.config.MongoDB)
	if err != nil {
		return err
	}
	f.mongodbManager = manager
	logger.Info(ctx, "MongoDB manager initialized")
	return nil
}

// initRedisManager 初始化 Redis 数据库管理器
func (f *Framework) initRedisManager(ctx context.Context) error {
	manager, err := redis.NewManager(f.config.Redis)
	if err != nil {
		return err
	}
	f.redisManager = manager
	logger.Info(ctx, "Redis manager initialized")
	return nil
}

// initTracing 初始化链路追踪
func (f *Framework) initTracing(ctx context.Context) error {
	if f.config.Tracing == nil {
		return nil
	}

	// 如果未设置服务名称，使用应用名称
	if f.config.Tracing.ServiceName == "" {
		f.config.Tracing.ServiceName = f.config.App.Name
	}
	if f.config.Tracing.ServiceName == "" {
		f.config.Tracing.ServiceName = "quickgo-service"
	}

	// 如果未设置服务版本，使用应用版本
	if f.config.Tracing.ServiceVersion == "" {
		f.config.Tracing.ServiceVersion = f.config.App.Version
	}
	if f.config.Tracing.ServiceVersion == "" {
		f.config.Tracing.ServiceVersion = "1.0.0"
	}

	// 如果未设置环境，使用应用环境
	if f.config.Tracing.Environment == "" {
		f.config.Tracing.Environment = f.config.App.Env
	}
	if f.config.Tracing.Environment == "" {
		f.config.Tracing.Environment = "development"
	}

	if err := tracing.Init(f.config.Tracing); err != nil {
		return fmt.Errorf("failed to initialize tracing: %w", err)
	}

	logger.Info(ctx, "Tracing initialized: service=%s, version=%s, environment=%s, otlp_enabled=%v, otlp_endpoint=%s",
		f.config.Tracing.ServiceName,
		f.config.Tracing.ServiceVersion,
		f.config.Tracing.Environment,
		f.config.Tracing.OTLP.Enabled,
		f.config.Tracing.OTLP.Endpoint,
	)

	return nil
}
