package quickgo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/team-dandelion/quickgo/grpc"
	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/metrics"
	"github.com/team-dandelion/quickgo/tracing"

	rpc "google.golang.org/grpc"

	"google.golang.org/grpc/keepalive"
)

const (
	defaultGrpcServerAddress          = "0.0.0.0"
	defaultGrpcServerPort             = 50051
	defaultGrpcServerKeepAliveTime    = 10 * time.Second
	defaultGrpcServerKeepAliveTimeout = 3 * time.Second
	defaultEtcdDialTimeout            = 5 * time.Second
)

type GrpcServerConfig struct {
	// 服务名称 示例：user-service
	ServiceName string `json:"serviceName" yaml:"serviceName" toml:"serviceName"`
	// 服务地址 示例：127.0.0.1:50051
	Address string `json:"address" yaml:"address" toml:"address"`
	// 注册到服务发现的地址，示例：10.0.0.12:50051。为空时按 SERVER_IP/本机地址推断。
	RegisterAddress string `json:"registerAddress" yaml:"registerAddress" toml:"registerAddress"`
	// 服务端口 示例：50051
	Port int `json:"port" yaml:"port" toml:"port"`
	// 最大连接空闲时间 示例：5s
	MaxConnectionIdle string `json:"maxConnectionIdle" yaml:"maxConnectionIdle" toml:"maxConnectionIdle"`
	// 最大连接年龄 示例：5s
	MaxConnectionAge string `json:"maxConnectionAge" yaml:"maxConnectionAge" toml:"maxConnectionAge"`
	// 最大连接年龄 grace time 示例：5s
	MaxConnectionAgeGrace string `json:"maxConnectionAgeGrace" yaml:"maxConnectionAgeGrace" toml:"maxConnectionAgeGrace"`
	// 心跳时间 示例：10s
	KeepAliveTime string `json:"keepAliveTime" yaml:"keepAliveTime" toml:"keepAliveTime"`
	// 心跳超时时间 示例：3s
	KeepAliveTimeout string `json:"keepAliveTimeout" yaml:"keepAliveTimeout" toml:"keepAliveTimeout"`
	// Etcd 配置（使用 etcd 服务发现时必需，全局共享）
	Etcd *EtcdConfig `json:"etcd" yaml:"etcd" toml:"etcd"`
	// Metrics 配置（可选）
	Metrics *metrics.Config `json:"metrics" yaml:"metrics" toml:"metrics"`
}

type EtcdConfig struct {
	Endpoints   []string `json:"endpoints" yaml:"endpoints" toml:"endpoints"`
	DialTimeout string   `json:"dialTimeout" yaml:"dialTimeout" toml:"dialTimeout"`
	Prefix      string   `json:"prefix" yaml:"prefix" toml:"prefix"`
	TTL         int64    `json:"ttl" yaml:"ttl" toml:"ttl"`
	Username    string   `json:"username" yaml:"username" toml:"username"`
	Password    string   `json:"password" yaml:"password" toml:"password"`
}

type GrpcServer struct {
	server    *grpc.Server
	config    *GrpcServerConfig
	registrar *grpc.ServiceRegistrar
}

type register func(s *rpc.Server)

func NewGrpcServer(config *GrpcServerConfig) (*GrpcServer, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}
	config = cloneGrpcServerConfig(config)
	applyGrpcServerDefaults(config)
	if err := validateGrpcServerConfig(config); err != nil {
		return nil, err
	}

	// etcd 配置是可选的；实际 registry 在 Start 阶段创建，避免构造期建立无用连接。
	if config.Etcd != nil {
		if _, err := parseDurationOrDefault(config.Etcd.DialTimeout, defaultEtcdDialTimeout); err != nil {
			logger.Error(context.Background(), "Failed to parse GrpcServerConfig.Etcd.DialTimeout: %v", err)
			return nil, err
		}
	} else {
		logger.Info(context.Background(), "Etcd not configured, running in standalone mode (no service discovery)")
	}

	keepTime, err := parseDurationOrDefault(config.KeepAliveTime, defaultGrpcServerKeepAliveTime)
	if err != nil {
		logger.Error(context.Background(), "Failed to parse GrpcServerConfig.Time: %v", err)
		return nil, err
	}

	timeout, err := parseDurationOrDefault(config.KeepAliveTimeout, defaultGrpcServerKeepAliveTimeout)
	if err != nil {
		logger.Error(context.Background(), "Failed to parse GrpcServerConfig.Timeout: %v", err)
		return nil, err
	}

	// 构建拦截器链
	unaryInterceptors := []rpc.UnaryServerInterceptor{
		grpc.LoggingInterceptor(),
		grpc.RecoveryInterceptor(),
	}
	streamInterceptors := []rpc.StreamServerInterceptor{
		grpc.StreamLoggingInterceptor(),
	}
	if config.Metrics != nil {
		m := metrics.New(*config.Metrics)
		unaryInterceptors = append(unaryInterceptors, metrics.UnaryServerInterceptor(m))
		streamInterceptors = append(streamInterceptors, metrics.StreamServerInterceptor(m))
	}

	// 如果启用了 OpenTelemetry tracing，添加 tracing 拦截器
	if tracing.IsEnabled() {
		unaryInterceptors = append([]rpc.UnaryServerInterceptor{tracing.UnaryServerInterceptor()}, unaryInterceptors...)
		streamInterceptors = append([]rpc.StreamServerInterceptor{tracing.StreamServerInterceptor()}, streamInterceptors...)
	}

	server, err := grpc.NewServer(grpc.Config{
		Address: config.Address,
		Port:    config.Port,
		Options: []rpc.ServerOption{
			rpc.ChainUnaryInterceptor(unaryInterceptors...),
			rpc.ChainStreamInterceptor(streamInterceptors...),
			// 添加keepalive配置
			rpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    keepTime,
				Timeout: timeout,
			}),
		},
	})

	if err != nil {
		logger.Error(context.Background(), "Failed to create grpc server: %v", err)
		return nil, err
	}

	return &GrpcServer{
		server: server,
		config: config,
	}, nil
}

func (s *GrpcServer) RegisterService(register register) error {
	register(s.server.GetServer())
	return nil
}

func (s *GrpcServer) Start() error {
	if s == nil || s.server == nil {
		return errors.New("grpc server is nil")
	}

	serverAddress := s.registerAddress()
	logger.Info(context.Background(), "Server will listen on %s:%d, register address: %s", s.config.Address, s.config.Port, serverAddress)

	if err := s.server.StartAsync(); err != nil {
		return fmt.Errorf("failed to start grpc server: %w", err)
	}

	// 如果没有配置 etcd，跳过服务注册
	if s.config.Etcd == nil {
		logger.Info(context.Background(), "Running in standalone mode, skipping service registration")
		return nil
	}

	dialTimeout, err := parseDurationOrDefault(s.config.Etcd.DialTimeout, defaultEtcdDialTimeout)
	if err != nil {
		return fmt.Errorf("failed to parse etcd dial timeout: %w", err)
	}

	etcdConfig := grpc.EtcdConfig{
		Endpoints:   s.config.Etcd.Endpoints,
		DialTimeout: dialTimeout,
		Prefix:      s.config.Etcd.Prefix,
		TTL:         s.config.Etcd.TTL,
		Username:    s.config.Etcd.Username,
		Password:    s.config.Etcd.Password,
	}

	registry, err := grpc.NewEtcdRegistry(etcdConfig)
	if err != nil {
		return s.rollbackStartedServer(fmt.Errorf("failed to create etcd registry: %w", err))
	}

	metadata := map[string]string{
		"version": "1.0.0",
		"weight":  "10",
		"region":  "default",
	}

	// 使用包含端口的完整地址创建新的 registrar
	s.registrar = grpc.NewServiceRegistrar(registry, s.config.ServiceName, serverAddress, metadata)

	if err := s.registrar.Register(context.Background()); err != nil {
		return s.rollbackStartedServer(fmt.Errorf("failed to register service to etcd: %w", err))
	}
	logger.Info(context.Background(), "Service registered to etcd: service=%s, address=%s", s.config.ServiceName, serverAddress)

	return nil
}

func (s *GrpcServer) rollbackStartedServer(startErr error) error {
	if s.registrar != nil {
		if err := s.registrar.Close(); err != nil {
			logger.Error(context.Background(), "Failed to close registrar during start rollback: %v", err)
			startErr = errors.Join(startErr, fmt.Errorf("close registrar during rollback: %w", err))
		}
		s.registrar = nil
	}
	if s.server != nil {
		if err := s.server.Stop(); err != nil {
			logger.Error(context.Background(), "Failed to stop grpc server during start rollback: %v", err)
			startErr = errors.Join(startErr, fmt.Errorf("stop grpc server during rollback: %w", err))
		}
	}
	return startErr
}

func (s *GrpcServer) Stop() error {
	if s == nil || s.server == nil {
		return nil
	}
	// 如果有 registrar（etcd 模式），需要注销服务
	if s.registrar != nil {
		if err := s.registrar.Deregister(context.Background()); err != nil {
			logger.Error(context.Background(), "Failed to deregister service: %v", err)
			return err
		}

		if err := s.registrar.Close(); err != nil {
			logger.Error(context.Background(), "Failed to close registrar: %v", err)
			return err
		}
		s.registrar = nil
	}

	if err := s.server.Stop(); err != nil {
		logger.Error(context.Background(), "Failed to stop server: %v", err)
		return err
	}
	return nil
}

func (s *GrpcServer) registerAddress() string {
	if s.config.RegisterAddress != "" {
		return s.config.RegisterAddress
	}
	serverIP := s.getLocalIP()
	if serverIP == "0.0.0.0" {
		serverIP = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", serverIP, s.config.Port)
}

// getLocalIP 获取本地 IP 地址
func (s *GrpcServer) getLocalIP() string {
	// 尝试从环境变量获取
	if ip := os.Getenv("SERVER_IP"); ip != "" {
		return ip
	}

	// 尝试连接外部地址以获取本地 IP
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func applyGrpcServerDefaults(config *GrpcServerConfig) {
	if config.Address == "" {
		config.Address = defaultGrpcServerAddress
	}
	if config.Port == 0 {
		config.Port = defaultGrpcServerPort
	}
}

func cloneGrpcServerConfig(config *GrpcServerConfig) *GrpcServerConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.Etcd != nil {
		etcd := *config.Etcd
		etcd.Endpoints = append([]string(nil), config.Etcd.Endpoints...)
		cloned.Etcd = &etcd
	}
	if config.Metrics != nil {
		metricsConfig := *config.Metrics
		if config.Metrics.Buckets != nil {
			metricsConfig.Buckets = append([]float64(nil), config.Metrics.Buckets...)
		}
		cloned.Metrics = &metricsConfig
	}
	return &cloned
}

func validateGrpcServerConfig(config *GrpcServerConfig) error {
	if config.Port < 0 || config.Port > 65535 {
		return fmt.Errorf("invalid grpc server port: %d", config.Port)
	}
	if config.Etcd == nil {
		return nil
	}
	if config.ServiceName == "" {
		return errors.New("grpc server serviceName is required when etcd is configured")
	}
	if len(config.Etcd.Endpoints) == 0 {
		return errors.New("grpc server etcd endpoints are required")
	}
	if config.Etcd.TTL < 0 {
		return fmt.Errorf("grpc server etcd ttl must be non-negative: %d", config.Etcd.TTL)
	}
	return nil
}

func parseDurationOrDefault(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}
