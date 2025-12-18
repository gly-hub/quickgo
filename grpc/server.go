package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/team-dandelion/quickgo/logger"
)

// Server gRPC服务器封装
type Server struct {
	server     *grpc.Server
	health     *health.Server
	address    string
	port       int
	listener   net.Listener
	options    []grpc.ServerOption
	services   []ServiceRegister
	reflection bool
}

// ServiceRegister 服务注册接口
type ServiceRegister func(*grpc.Server)

// Config gRPC服务器配置
type Config struct {
	Address    string
	Port       int
	Options    []grpc.ServerOption
	Reflection bool // 是否启用反射（用于调试）
}

// NewServer 创建新的gRPC服务器实例
func NewServer(config Config) (*Server, error) {
	if config.Address == "" {
		config.Address = "0.0.0.0"
	}
	if config.Port == 0 {
		config.Port = 50051
	}

	s := &Server{
		address:    config.Address,
		port:       config.Port,
		options:    config.Options,
		services:   make([]ServiceRegister, 0),
		reflection: config.Reflection,
	}

	// 创建health检查服务
	s.health = health.NewServer()

	// 创建gRPC服务器
	s.server = grpc.NewServer(s.options...)

	// 注册健康检查服务
	grpc_health_v1.RegisterHealthServer(s.server, s.health)

	// 如果启用反射，注册反射服务
	if s.reflection {
		reflection.Register(s.server)
	}

	return s, nil
}

// RegisterService 注册gRPC服务
func (s *Server) RegisterService(register ServiceRegister) {
	s.services = append(s.services, register)
	register(s.server)
}

// Start 启动gRPC服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.address, s.port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", addr, err)
	}
	s.listener = listener

	// 设置所有服务为健康状态
	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	ctx := context.Background()
	logger.Info(ctx, "gRPC server starting on %s", addr)

	// 启动服务器
	if err := s.server.Serve(listener); err != nil {
		logger.Error(ctx, "gRPC server failed to serve: %v", err)
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

// StartAsync 异步启动gRPC服务器
func (s *Server) StartAsync() error {
	addr := fmt.Sprintf("%s:%d", s.address, s.port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", addr, err)
	}
	s.listener = listener

	// 设置所有服务为健康状态
	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	ctx := context.Background()
	logger.Info(ctx, "gRPC server starting on %s", addr)

	// 在goroutine中启动服务器
	go func() {
		if err := s.server.Serve(listener); err != nil {
			logger.Error(ctx, "gRPC server error: %v", err)
		}
	}()

	return nil
}

// Stop 停止gRPC服务器
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	// 设置服务为不健康状态
	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// 优雅关闭
	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()

	// 设置超时
	ctx := context.Background()
	t := time.NewTimer(10 * time.Second)
	select {
	case <-t.C:
		// 超时后强制停止
		s.server.Stop()
		logger.Warn(ctx, "gRPC server forcefully stopped")
	case <-stopped:
		t.Stop()
		logger.Info(ctx, "gRPC server gracefully stopped")
	}

	return nil
}

// StopWithContext 使用context停止gRPC服务器
func (s *Server) StopWithContext(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	// 设置服务为不健康状态
	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// 优雅关闭
	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		// 上下文取消后强制停止
		s.server.Stop()
		logger.Warn(ctx, "gRPC server forcefully stopped due to context cancellation")
		return ctx.Err()
	case <-stopped:
		logger.Info(ctx, "gRPC server gracefully stopped")
		return nil
	}
}

// GetServer 获取底层grpc.Server实例
func (s *Server) GetServer() *grpc.Server {
	return s.server
}

// GetAddress 获取服务器地址
func (s *Server) GetAddress() string {
	return fmt.Sprintf("%s:%d", s.address, s.port)
}

// SetHealthStatus 设置服务健康状态
func (s *Server) SetHealthStatus(service string, status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	s.health.SetServingStatus(service, status)
}

// IsRunning 检查服务器是否正在运行
func (s *Server) IsRunning() bool {
	return s.listener != nil
}
