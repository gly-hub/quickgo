package quickgo

import (
	"context"
	"errors"
	"fmt"
	"github.com/team-dandelion/quickgo/grpc"
	"github.com/team-dandelion/quickgo/logger"
	"github.com/team-dandelion/quickgo/tracing"
	"net"
	"os"
	"time"

	rpc "google.golang.org/grpc"

	"google.golang.org/grpc/keepalive"
)

type GrpcServerConfig struct {
	// 服务名称 示例：user-service
	ServiceName string `json:"serviceName" yaml:"serviceName" toml:"serviceName"`
	// 服务地址 示例：127.0.0.1:50051
	Address string `json:"address" yaml:"address" toml:"address"`
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
	if config.Etcd == nil {
		return nil, errors.New("etcd config is nil")
	}

	dialTimeout, err := time.ParseDuration(config.Etcd.DialTimeout)
	if err != nil {
		logger.Error(context.Background(), "Failed to parse GrpcServerConfig.Etcd.DialTimeout: %v", err)
		return nil, err
	}
	// 创建 etcd 注册中心配置
	etcdConfig := grpc.EtcdConfig{
		Endpoints:   config.Etcd.Endpoints,
		DialTimeout: dialTimeout,
		Prefix:      config.Etcd.Prefix,
		TTL:         config.Etcd.TTL,
		Username:    config.Etcd.Username,
		Password:    config.Etcd.Password,
	}

	// 创建 etcd registry
	registry, err := grpc.NewEtcdRegistry(etcdConfig)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create etcd registry: %v", err)
	}
	// 创建服务注册器
	metadata := map[string]string{
		"version": "1.0.0",
		"weight":  "10",
		"region":  "default",
	}
	registrar := grpc.NewServiceRegistrar(registry, config.ServiceName,
		config.Address, metadata)

	keepTime, err := time.ParseDuration(config.KeepAliveTime)
	if err != nil {
		logger.Error(context.Background(), "Failed to parse GrpcServerConfig.Time: %v", err)
		return nil, err
	}

	timeout, err := time.ParseDuration(config.KeepAliveTimeout)
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

	// 如果启用了 OpenTelemetry tracing，添加 tracing 拦截器
	if tracing.IsEnabled() {
		unaryInterceptors = append([]rpc.UnaryServerInterceptor{tracing.UnaryServerInterceptor()}, unaryInterceptors...)
		streamInterceptors = append([]rpc.StreamServerInterceptor{tracing.StreamServerInterceptor()}, streamInterceptors...)
	}

	server, err := grpc.NewServer(grpc.Config{
		Address: "0.0.0.0",
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
		server:    server,
		config:    config,
		registrar: registrar,
	}, nil
}

func (s *GrpcServer) RegisterService(register register) error {
	register(s.server.GetServer())
	return nil
}

func (s *GrpcServer) Start() error {
	// 获取服务器地址（用于注册到 etcd）
	// 注意：不能使用 0.0.0.0，因为客户端无法连接到 0.0.0.0
	// 需要使用实际可访问的 IP 地址
	serverIP := s.getLocalIP()
	if serverIP == "0.0.0.0" {
		// 如果获取到 0.0.0.0，使用 127.0.0.1（本地开发环境）
		serverIP = "127.0.0.1"
	}
	serverAddress := fmt.Sprintf("%s:%d", serverIP, s.config.Port)
	logger.Info(context.Background(), "Server will listen on %s:%d, register address: %s", s.config.Address, s.config.Port, serverAddress)

	// 启动服务器（异步）
	go func() {
		logger.Info(context.Background(), "Starting gRPC server on %s:%d", s.config.Address, s.config.Port)
		if err := s.server.Start(); err != nil {
			logger.Fatal(context.Background(), "Failed to start server: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(500 * time.Millisecond)

	// 使用正确的地址注册服务（包含端口）
	// 需要重新创建 registrar，因为创建时使用的是 config.Address（0.0.0.0），缺少端口
	// 先关闭旧的 registrar（如果存在）
	if s.registrar != nil {
		s.registrar.Close()
	}

	if s.config.Etcd != nil {
		dialTimeout, err := time.ParseDuration(s.config.Etcd.DialTimeout)
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
			return fmt.Errorf("failed to create etcd registry: %w", err)
		}

		metadata := map[string]string{
			"version": "1.0.0",
			"weight":  "10",
			"region":  "default",
		}

		// 使用包含端口的完整地址创建新的 registrar
		s.registrar = grpc.NewServiceRegistrar(registry, s.config.ServiceName, serverAddress, metadata)
	}

	if err := s.registrar.Register(context.Background()); err != nil {
		logger.Fatal(context.Background(), "Failed to register service to etcd: %v", err)
	}
	logger.Info(context.Background(), "Service registered to etcd: service=%s, address=%s", s.config.ServiceName, serverAddress)

	// 启动心跳保持
	s.registrar.StartKeepAlive(20 * time.Second)
	return nil
}

func (s *GrpcServer) Stop() error {
	if err := s.registrar.Deregister(context.Background()); err != nil {
		logger.Error(context.Background(), "Failed to deregister service: %v", err)
		return err
	}

	if err := s.registrar.Close(); err != nil {
		logger.Error(context.Background(), "Failed to close registrar: %v", err)
		return err
	}

	if err := s.server.Stop(); err != nil {
		logger.Error(context.Background(), "Failed to stop server: %v", err)
		return err
	}
	return nil
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
