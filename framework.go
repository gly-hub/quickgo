package quickgo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/team-dandelion/quickgo/db/gorm"
	"github.com/team-dandelion/quickgo/db/mongodb"
	"github.com/team-dandelion/quickgo/db/redis"
	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/metrics"
	"github.com/team-dandelion/quickgo/tracing"
)

// Framework 主体框架，统一管理所有组件
type Framework struct {
	// 配置
	config *FrameworkConfig

	// 核心组件
	logger  *logger.Logger
	metrics *metrics.Metrics

	// 服务组件
	grpcServer    *GrpcServer
	grpcClientMgr *GrpcClientManager
	httpServer    *HTTPServer

	// 数据库组件
	gormManager    *gorm.Manager
	mongodbManager *mongodb.Manager
	redisManager   *redis.Manager

	// 组件注册表（用于扩展）
	components                map[string]Component
	componentOrder            []string
	initializedComponentOrder []string

	// 生命周期管理
	mu                 sync.RWMutex
	lifecycleMu        sync.Mutex
	initializing       bool
	stopping           bool
	initialized        bool
	started            bool
	stopped            bool
	tracingInitialized bool
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

	// 指标配置（可选）
	Metrics *metrics.Config
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
	Enabled      bool   `json:"enabled" yaml:"enabled" toml:"enabled"`                // 是否启用
	Level        string `json:"level" yaml:"level" toml:"level"`                      // 日志级别：debug, info, warn, error
	Output       string `json:"output" yaml:"output" toml:"output"`                   // 输出方式：console, file
	File         string `json:"file" yaml:"file" toml:"file"`                         // 文件路径（output=file 时）
	Service      string `json:"service" yaml:"service" toml:"service"`                // 服务名称
	Version      string `json:"version" yaml:"version" toml:"version"`                // 服务版本
	EnableCaller bool   `json:"enableCaller" yaml:"enableCaller" toml:"enableCaller"` // 是否记录调用者位置，默认 false
	Synchronous  bool   `json:"synchronous" yaml:"synchronous" toml:"synchronous"`    // 是否同步写出日志，默认 false
	BufferSize   int    `json:"bufferSize" yaml:"bufferSize" toml:"bufferSize"`       // 异步日志队列长度，默认 1024
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
		config:         config,
		components:     make(map[string]Component),
		componentOrder: make([]string, 0),
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
		if config == nil {
			c.Tracing = nil
			return
		}
		cloned := *config
		c.Tracing = &cloned
	}
}

// ConfigOptionWithMetrics 配置指标采集
func ConfigOptionWithMetrics(config *metrics.Config) FrameworkOption {
	return func(c *FrameworkConfig) {
		c.Metrics = cloneMetricsConfig(config)
	}
}

func cloneMetricsConfig(config *metrics.Config) *metrics.Config {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.Buckets != nil {
		cloned.Buckets = append([]float64(nil), config.Buckets...)
	}
	return &cloned
}

// Init 初始化所有组件
// 只初始化通过 Option 显式配置的组件
func (f *Framework) Init() error {
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()

	f.mu.Lock()
	if f.initialized {
		f.mu.Unlock()
		return errors.New("framework already initialized")
	}
	f.stopped = false
	f.initializing = true
	f.mu.Unlock()

	ctx := context.Background()
	initialized := false
	defer func() {
		if !initialized {
			if err := f.stop(ctx, false); err != nil {
				logger.Error(ctx, "Failed to rollback framework after init failure: %v", err)
			}
			f.mu.Lock()
			f.initializing = false
			f.mu.Unlock()
		}
	}()

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
		// 保持全局 logger 可用，但使用高于 Fatal 的级别实现真正静默。
		logger.Init(logger.Config{
			Level:         logger.LevelFatal + 1,
			DisableCaller: true,
		})
		f.setLogger(logger.GetDefault())
	}

	// 3. 初始化指标收集器（如果配置）
	if f.config.Metrics != nil {
		f.setMetrics(metrics.New(*f.config.Metrics))
	}

	// 4. 初始化 gRPC Server（仅当通过 Option 配置时）
	if f.config.GrpcServer != nil {
		if f.config.Metrics != nil && f.config.GrpcServer.Metrics == nil {
			config := *f.config.GrpcServer
			config.Metrics = cloneMetricsConfig(f.config.Metrics)
			f.config.GrpcServer = &config
		}
		if f.metrics != nil {
			config := *f.config.GrpcServer
			config.metrics = f.metrics
			f.config.GrpcServer = &config
		}
		if err := f.initGrpcServer(ctx); err != nil {
			return fmt.Errorf("failed to init grpc server: %w", err)
		}
	}

	// 5. 初始化 gRPC Client Manager（仅当通过 Option 配置时）
	if f.config.GrpcClient != nil {
		if err := f.initGrpcClientManager(ctx); err != nil {
			return fmt.Errorf("failed to init grpc client manager: %w", err)
		}
	}

	// 6. 初始化 HTTP Server（仅当通过 Option 配置时）
	if f.config.HTTPServer != nil && f.config.HTTPServer.Enabled {
		if f.config.Metrics != nil && f.config.HTTPServer.Metrics == nil {
			config := *f.config.HTTPServer
			config.Metrics = cloneMetricsConfig(f.config.Metrics)
			f.config.HTTPServer = &config
		}
		if f.metrics != nil {
			config := *f.config.HTTPServer
			config.metrics = f.metrics
			f.config.HTTPServer = &config
		}
		if err := f.initHTTPServer(ctx); err != nil {
			return fmt.Errorf("failed to init http server: %w", err)
		}
	}

	// 7. 初始化 GORM 数据库管理器（仅当通过 Option 配置时）
	if f.config.Gorm != nil {
		if err := f.initGormManager(ctx); err != nil {
			return fmt.Errorf("failed to init gorm manager: %w", err)
		}
	}

	// 8. 初始化 MongoDB 数据库管理器（仅当通过 Option 配置时）
	if f.config.MongoDB != nil {
		if err := f.initMongoDBManager(ctx); err != nil {
			return fmt.Errorf("failed to init mongodb manager: %w", err)
		}
	}

	// 9. 初始化 Redis 数据库管理器（仅当通过 Option 配置时）
	if f.config.Redis != nil {
		if err := f.initRedisManager(ctx); err != nil {
			return fmt.Errorf("failed to init redis manager: %w", err)
		}
	}

	// 10. 初始化自定义组件
	for _, entry := range f.componentsSnapshot() {
		component := entry.component
		if component != nil && component.IsEnabled() {
			if err := component.Init(ctx); err != nil {
				return fmt.Errorf("failed to init component %s: %w", component.Name(), err)
			}
			f.mu.Lock()
			f.initializedComponentOrder = append(f.initializedComponentOrder, entry.name)
			f.mu.Unlock()
		}
	}

	f.mu.Lock()
	f.initialized = true
	f.initializing = false
	f.mu.Unlock()
	initialized = true
	logger.Info(ctx, "Framework initialized successfully")
	return nil
}

// Start 启动所有组件
func (f *Framework) Start() error {
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()

	f.mu.Lock()
	if !f.initialized {
		f.mu.Unlock()
		return errors.New("framework not initialized, call Init() first")
	}

	if f.started {
		f.mu.Unlock()
		return errors.New("framework already started")
	}
	grpcServer := f.grpcServer
	httpServer := f.httpServer
	grpcClientMgr := f.grpcClientMgr
	components := f.initializedComponentsLocked()
	f.mu.Unlock()

	ctx := context.Background()
	startFailed := func(startErr error) error {
		if rollbackErr := f.stop(ctx, false); rollbackErr != nil {
			return errors.Join(startErr, fmt.Errorf("rollback framework after start failure: %w", rollbackErr))
		}
		return startErr
	}

	// 1. 启动 gRPC Server
	if grpcServer != nil {
		if err := grpcServer.Start(); err != nil {
			return startFailed(fmt.Errorf("failed to start grpc server: %w", err))
		}
		logger.Info(ctx, "gRPC server started")
	}

	// 2. 启动 HTTP Server
	if httpServer != nil {
		if err := httpServer.StartAsync(); err != nil {
			return startFailed(fmt.Errorf("failed to start http server: %w", err))
		}
		logger.Info(ctx, "HTTP server started")
	}

	// 3. 启动自定义组件
	for _, component := range components {
		if component != nil && component.IsEnabled() {
			if err := component.Start(ctx); err != nil {
				return startFailed(fmt.Errorf("failed to start component %s: %w", component.Name(), err))
			}
		}
	}

	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return startFailed(errors.New("framework already started"))
	}
	f.started = true
	f.mu.Unlock()
	if grpcClientMgr != nil {
		grpcClientMgr.StartHealthCheck()
	}
	logger.Info(ctx, "Framework started successfully")
	return nil
}

// Stop 停止所有组件
func (f *Framework) Stop() error {
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()

	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return nil // 已停止
	}
	if !f.initializing && !f.initialized && !f.started {
		f.mu.Unlock()
		return nil
	}
	f.mu.Unlock()

	ctx := context.Background()
	return f.stop(ctx, true)
}

func (f *Framework) stop(ctx context.Context, logStopped bool) error {
	f.mu.Lock()
	if f.stopped || (!f.initializing && !f.initialized && !f.started) {
		f.mu.Unlock()
		return nil
	}
	components := f.initializedComponentEntriesLocked()
	httpServer := f.httpServer
	grpcServer := f.grpcServer
	grpcClientMgr := f.grpcClientMgr
	redisManager := f.redisManager
	mongodbManager := f.mongodbManager
	gormManager := f.gormManager
	frameworkLogger := f.logger
	traceEnabled := f.tracingInitialized
	f.stopping = true
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.stopping = false
		f.mu.Unlock()
	}()

	var errs []error

	// 按相反顺序停止组件

	// 1. 停止自定义组件
	failedComponents := make(map[string]struct{})
	for i := len(components) - 1; i >= 0; i-- {
		entry := components[i]
		component := entry.component
		if component != nil {
			if err := component.Stop(ctx); err != nil {
				logger.Error(ctx, "Failed to stop component %s: %v", component.Name(), err)
				errs = append(errs, fmt.Errorf("component %s: %w", component.Name(), err))
				failedComponents[entry.name] = struct{}{}
			}
		}
	}

	// 2. 停止 HTTP Server
	httpServerClosed := httpServer == nil
	if httpServer != nil {
		if err := httpServer.Stop(); err != nil {
			logger.Error(ctx, "Failed to stop http server: %v", err)
			errs = append(errs, fmt.Errorf("http server: %w", err))
		} else {
			httpServerClosed = true
		}
	}

	// 3. 停止 gRPC Server
	grpcServerClosed := grpcServer == nil
	if grpcServer != nil {
		if err := grpcServer.Stop(); err != nil {
			logger.Error(ctx, "Failed to stop grpc server: %v", err)
			errs = append(errs, fmt.Errorf("grpc server: %w", err))
		} else {
			grpcServerClosed = true
		}
	}

	// 4. 关闭 gRPC Client Manager
	grpcClientMgrClosed := grpcClientMgr == nil
	if grpcClientMgr != nil {
		if err := grpcClientMgr.CloseAll(); err != nil {
			logger.Error(ctx, "Failed to close grpc client manager: %v", err)
			errs = append(errs, fmt.Errorf("grpc client manager: %w", err))
		} else {
			grpcClientMgrClosed = true
		}
	}

	// 5. 关闭数据库连接
	redisManagerClosed := redisManager == nil
	if redisManager != nil {
		if err := redisManager.Close(); err != nil {
			logger.Error(ctx, "Failed to close redis manager: %v", err)
			errs = append(errs, fmt.Errorf("redis manager: %w", err))
		} else {
			redisManagerClosed = true
		}
	}

	mongodbManagerClosed := mongodbManager == nil
	if mongodbManager != nil {
		if err := mongodbManager.Close(); err != nil {
			logger.Error(ctx, "Failed to close mongodb manager: %v", err)
			errs = append(errs, fmt.Errorf("mongodb manager: %w", err))
		} else {
			mongodbManagerClosed = true
		}
	}

	gormManagerClosed := gormManager == nil
	if gormManager != nil {
		if err := gormManager.Close(); err != nil {
			logger.Error(ctx, "Failed to close gorm manager: %v", err)
			errs = append(errs, fmt.Errorf("gorm manager: %w", err))
		} else {
			gormManagerClosed = true
		}
	}

	// 关闭链路追踪
	tracingClosed := !traceEnabled
	if traceEnabled {
		if err := tracing.Shutdown(ctx); err != nil {
			logger.Error(ctx, "Failed to shutdown tracing: %v", err)
			errs = append(errs, fmt.Errorf("tracing: %w", err))
		} else {
			tracingClosed = true
			logger.Info(ctx, "Tracing shutdown successfully")
		}
	}

	if logStopped {
		logger.Info(ctx, "Framework stopped")
	}
	loggerClosed := frameworkLogger == nil
	if frameworkLogger != nil {
		closedGlobal, err := logger.CloseIfDefault(frameworkLogger)
		if !closedGlobal {
			err = frameworkLogger.Close()
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("logger: %w", err))
		} else {
			loggerClosed = true
		}
	}

	f.mu.Lock()
	if httpServerClosed {
		f.httpServer = nil
	}
	if grpcServerClosed {
		f.grpcServer = nil
	}
	if grpcClientMgrClosed {
		f.grpcClientMgr = nil
	}
	if redisManagerClosed {
		f.redisManager = nil
	}
	if mongodbManagerClosed {
		f.mongodbManager = nil
	}
	if gormManagerClosed {
		f.gormManager = nil
	}
	if loggerClosed {
		f.logger = nil
	}
	if tracingClosed {
		f.tracingInitialized = false
	}
	f.started = false
	f.initializing = false
	if len(errs) == 0 {
		f.initialized = false
		f.initializedComponentOrder = nil
		f.metrics = nil
		f.stopped = true
	} else {
		remaining := make([]string, 0, len(failedComponents))
		for _, entry := range components {
			if _, failed := failedComponents[entry.name]; failed {
				remaining = append(remaining, entry.name)
			}
		}
		f.initializedComponentOrder = remaining
		f.initialized = true
		f.stopped = false
	}
	f.mu.Unlock()
	if len(errs) > 0 {
		return fmt.Errorf("framework stopped with errors: %w", errors.Join(errs...))
	}
	return nil
}

// Wait 等待中断信号（优雅关闭）
func (f *Framework) Wait() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
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
	if f.initializing || f.initialized || f.started || f.stopping {
		return errors.New("cannot register component after framework initialization has started")
	}

	f.components[name] = component
	f.componentOrder = append(f.componentOrder, name)
	logger.Info(context.Background(), "Component registered: %s", name)
	return nil
}

type componentEntry struct {
	name      string
	component Component
}

func (f *Framework) componentsSnapshot() []componentEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()

	components := make([]componentEntry, 0, len(f.componentOrder))
	for _, name := range f.componentOrder {
		if component := f.components[name]; component != nil {
			components = append(components, componentEntry{name: name, component: component})
		}
	}
	return components
}

func (f *Framework) initializedComponentsLocked() []Component {
	components := make([]Component, 0, len(f.initializedComponentOrder))
	for _, name := range f.initializedComponentOrder {
		if component := f.components[name]; component != nil {
			components = append(components, component)
		}
	}
	return components
}

func (f *Framework) initializedComponentEntriesLocked() []componentEntry {
	components := make([]componentEntry, 0, len(f.initializedComponentOrder))
	for _, name := range f.initializedComponentOrder {
		if component := f.components[name]; component != nil {
			components = append(components, componentEntry{name: name, component: component})
		}
	}
	return components
}

func (f *Framework) setLogger(value *logger.Logger) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logger = value
}

func (f *Framework) setMetrics(value *metrics.Metrics) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metrics = value
}

func (f *Framework) setGrpcServer(value *GrpcServer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grpcServer = value
}

func (f *Framework) setGrpcClientManager(value *GrpcClientManager) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.grpcClientMgr = value
}

func (f *Framework) setHTTPServer(value *HTTPServer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.httpServer = value
}

func (f *Framework) setGormManager(value *gorm.Manager) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gormManager = value
}

func (f *Framework) setMongoDBManager(value *mongodb.Manager) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mongodbManager = value
}

func (f *Framework) setRedisManager(value *redis.Manager) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redisManager = value
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
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.logger
}

// GrpcServer 获取 gRPC 服务器实例
func (f *Framework) GrpcServer() *GrpcServer {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.grpcServer
}

// GrpcClientManager 获取 gRPC 客户端管理器实例
func (f *Framework) GrpcClientManager() *GrpcClientManager {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.grpcClientMgr
}

// HTTPServer 获取 HTTP 服务器实例
func (f *Framework) HTTPServer() *HTTPServer {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.httpServer
}

// GormManager 获取 GORM 数据库管理器实例
func (f *Framework) GormManager() *gorm.Manager {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.gormManager
}

// MongoManager 获取 MongoDB 数据库管理器实例
func (f *Framework) MongoManager() *mongodb.Manager {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mongodbManager
}

// RedisManager 获取 Redis 数据库管理器实例
func (f *Framework) RedisManager() *redis.Manager {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.redisManager
}

// Metrics 获取框架共享的指标收集器。
func (f *Framework) Metrics() *metrics.Metrics {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.metrics
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
		Level:         level,
		Service:       cfg.Service,
		Version:       cfg.Version,
		EnableCaller:  cfg.EnableCaller,
		DisableCaller: !cfg.EnableCaller,
		Async:         !cfg.Synchronous,
		BufferSize:    cfg.BufferSize,
	}

	// 设置输出方式
	if cfg.Output == "file" && cfg.File != "" {
		loggerConfig.Output = cfg.File
	}

	if err := logger.Init(loggerConfig); err != nil {
		return err
	}

	f.setLogger(logger.GetDefault())
	return nil
}

// initGrpcServer 初始化 gRPC 服务器
func (f *Framework) initGrpcServer(ctx context.Context) error {
	server, err := NewGrpcServer(f.config.GrpcServer)
	if err != nil {
		return err
	}

	f.setGrpcServer(server)
	return nil
}

// initGrpcClientManager 初始化 gRPC 客户端管理器
func (f *Framework) initGrpcClientManager(ctx context.Context) error {
	manager, err := NewGrpcClientManager(f.config.GrpcClient)
	if err != nil {
		return err
	}

	f.setGrpcClientManager(manager)
	return nil
}

// initHTTPServer 初始化 HTTP 服务器
func (f *Framework) initHTTPServer(ctx context.Context) error {
	server, err := NewHTTPServer(f.config.HTTPServer)
	if err != nil {
		return err
	}

	f.setHTTPServer(server)
	return nil
}

// initGormManager 初始化 GORM 数据库管理器
func (f *Framework) initGormManager(ctx context.Context) error {
	manager, err := gorm.NewManager(f.config.Gorm)
	if err != nil {
		return err
	}
	f.setGormManager(manager)
	logger.Info(ctx, "GORM manager initialized")
	return nil
}

// initMongoDBManager 初始化 MongoDB 数据库管理器
func (f *Framework) initMongoDBManager(ctx context.Context) error {
	manager, err := mongodb.NewManager(f.config.MongoDB)
	if err != nil {
		return err
	}
	f.setMongoDBManager(manager)
	logger.Info(ctx, "MongoDB manager initialized")
	return nil
}

// initRedisManager 初始化 Redis 数据库管理器
func (f *Framework) initRedisManager(ctx context.Context) error {
	manager, err := redis.NewManager(f.config.Redis)
	if err != nil {
		return err
	}
	f.setRedisManager(manager)
	logger.Info(ctx, "Redis manager initialized")
	return nil
}

// initTracing 初始化链路追踪
func (f *Framework) initTracing(ctx context.Context) error {
	if f.config.Tracing == nil {
		return nil
	}
	cfg := *f.config.Tracing

	// 如果未设置服务名称，使用应用名称
	if cfg.ServiceName == "" {
		cfg.ServiceName = f.config.App.Name
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "quickgo-service"
	}

	// 如果未设置服务版本，使用应用版本
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = f.config.App.Version
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "1.0.0"
	}

	// 如果未设置环境，使用应用环境
	if cfg.Environment == "" {
		cfg.Environment = f.config.App.Env
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	if err := tracing.Init(&cfg); err != nil {
		return fmt.Errorf("failed to initialize tracing: %w", err)
	}
	f.config.Tracing = &cfg
	f.mu.Lock()
	f.tracingInitialized = cfg.Enabled
	f.mu.Unlock()

	logger.Info(ctx, "Tracing initialized: service=%s, version=%s, environment=%s, otlp_enabled=%v, otlp_endpoint=%s",
		cfg.ServiceName,
		cfg.ServiceVersion,
		cfg.Environment,
		cfg.OTLP.Enabled,
		cfg.OTLP.Endpoint,
	)

	return nil
}
