package main

import (
	"context"
	"fmt"
	hello2 "gly-hub/go-dandelion/quickgo/example/grpc/proto/gen/proto"
	"gly-hub/go-dandelion/quickgo/grpc"
	"gly-hub/go-dandelion/quickgo/logger"

	rpc "google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// 服务配置
	serviceName = "hello-service"
	servicePort = 50051
	// etcd 配置
	defaultEtcdEndpoint = "127.0.0.1:2379"
	etcdPrefix          = "/grpc/services"
	etcdTTL             = 30
)

// HelloServer 实现 HelloService
type HelloServer struct {
	hello2.UnimplementedHelloServiceServer
}

// SayHello 简单问候
func (s *HelloServer) SayHello(ctx context.Context, req *hello2.HelloRequest) (*hello2.HelloResponse, error) {
	logger.Info(ctx, "Received SayHello request: name=%s, message=%s", req.Name, req.Message)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	greeting := fmt.Sprintf("Hello, %s! Your message: %s", req.Name, req.Message)

	return &hello2.HelloResponse{
		Greeting:  greeting,
		Timestamp: time.Now().Format(time.RFC3339),
		Code:      200,
		Message:   "success",
	}, nil
}

// SayHelloStream 服务端流式问候
func (s *HelloServer) SayHelloStream(req *hello2.HelloRequest, stream hello2.HelloService_SayHelloStreamServer) error {
	ctx := stream.Context()
	logger.Info(ctx, "Received SayHelloStream request: name=%s", req.Name)

	if req.Name == "" {
		return status.Error(codes.InvalidArgument, "name is required")
	}

	// 发送 5 条流式响应
	for i := 1; i <= 5; i++ {
		greeting := fmt.Sprintf("Hello %s, this is message #%d", req.Name, i)

		resp := &hello2.HelloResponse{
			Greeting:  greeting,
			Timestamp: time.Now().Format(time.RFC3339),
			Code:      200,
			Message:   fmt.Sprintf("stream message %d", i),
		}

		if err := stream.Send(resp); err != nil {
			logger.Error(ctx, "Failed to send stream message: %v", err)
			return err
		}

		logger.Info(ctx, "Sent stream message #%d", i)
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// SayHelloClientStream 客户端流式问候
func (s *HelloServer) SayHelloClientStream(stream hello2.HelloService_SayHelloClientStreamServer) error {
	ctx := stream.Context()
	logger.Info(ctx, "Received SayHelloClientStream request")

	var names []string
	var messages []string

	// 接收客户端流式请求
	for {
		req, err := stream.Recv()
		if err != nil {
			// 流结束
			break
		}

		logger.Info(ctx, "Received client stream message: name=%s, message=%s", req.Name, req.Message)
		names = append(names, req.Name)
		messages = append(messages, req.Message)
	}

	// 汇总并返回响应
	greeting := fmt.Sprintf("Received %d messages from: %v", len(names), names)

	return stream.SendAndClose(&hello2.HelloResponse{
		Greeting:  greeting,
		Timestamp: time.Now().Format(time.RFC3339),
		Code:      200,
		Message:   fmt.Sprintf("processed %d messages", len(messages)),
	})
}

// SayHelloBidiStream 双向流式问候
func (s *HelloServer) SayHelloBidiStream(stream hello2.HelloService_SayHelloBidiStreamServer) error {
	ctx := stream.Context()
	logger.Info(ctx, "Received SayHelloBidiStream request")

	// 接收并响应流式消息
	for {
		req, err := stream.Recv()
		if err != nil {
			// 流结束
			break
		}

		logger.Info(ctx, "Received bidi stream message: name=%s, message=%s", req.Name, req.Message)

		// 立即响应
		greeting := fmt.Sprintf("Echo: Hello %s! Your message: %s", req.Name, req.Message)

		resp := &hello2.HelloResponse{
			Greeting:  greeting,
			Timestamp: time.Now().Format(time.RFC3339),
			Code:      200,
			Message:   "echo response",
		}

		if err := stream.Send(resp); err != nil {
			logger.Error(ctx, "Failed to send bidi stream response: %v", err)
			return err
		}
	}

	return nil
}

func main() {
	ctx := context.Background()

	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
		//Output: logger.OutputConsole,
	})

	// 获取 etcd 端点（可以从环境变量读取）
	etcdEndpoint := os.Getenv("ETCD_ENDPOINT")
	if etcdEndpoint == "" {
		etcdEndpoint = defaultEtcdEndpoint
	}

	// 创建 etcd 注册中心配置
	etcdConfig := grpc.EtcdConfig{
		Endpoints:   []string{etcdEndpoint},
		DialTimeout: 5 * time.Second,
		Prefix:      etcdPrefix,
		TTL:         etcdTTL,
	}

	// 创建 etcd registry
	registry, err := grpc.NewEtcdRegistry(etcdConfig)
	if err != nil {
		logger.Fatal(ctx, "Failed to create etcd registry: %v", err)
	}
	defer registry.Close()

	// 创建 gRPC 服务器配置
	config := grpc.Config{
		Address: "0.0.0.0",
		Port:    servicePort,
		Options: []rpc.ServerOption{
			grpc.ChainUnaryInterceptors(
				grpc.LoggingInterceptor(),
				grpc.RecoveryInterceptor(),
			),
			grpc.ChainStreamInterceptors(
				grpc.StreamLoggingInterceptor(),
			),
			// 添加keepalive配置
			rpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    10,
				Timeout: 3,
			}),
		},
	}

	// 创建服务器
	server, err := grpc.NewServer(config)
	if err != nil {
		logger.Fatal(ctx, "Failed to create server: %v", err)
	}

	// 注册 gRPC 服务
	helloServer := &HelloServer{}
	hello2.RegisterHelloServiceServer(server.GetServer(), helloServer)

	// 获取服务器地址（用于注册到 etcd）
	serverAddress := fmt.Sprintf("%s:%d", getLocalIP(), servicePort)
	logger.Info(ctx, "Server will listen on %s", serverAddress)

	// 创建服务注册器
	metadata := map[string]string{
		"version": "1.0.0",
		"weight":  "10",
		"region":  "default",
	}
	registrar := grpc.NewServiceRegistrar(registry, serviceName, serverAddress, metadata)

	// 启动服务器（异步）
	go func() {
		logger.Info(ctx, "Starting gRPC server on %s:%d", config.Address, config.Port)
		if err := server.Start(); err != nil {
			logger.Fatal(ctx, "Failed to start server: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(500 * time.Millisecond)

	// 注册服务到 etcd
	if err := registrar.Register(ctx); err != nil {
		logger.Fatal(ctx, "Failed to register service to etcd: %v", err)
	}
	logger.Info(ctx, "Service registered to etcd: service=%s, address=%s", serviceName, serverAddress)

	// 启动心跳保持
	registrar.StartKeepAlive(20 * time.Second)

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info(ctx, "Shutting down server...")

	// 注销服务
	if err := registrar.Deregister(ctx); err != nil {
		logger.Error(ctx, "Failed to deregister service: %v", err)
	}
	registrar.Close()

	// 停止服务器
	if err := server.Stop(); err != nil {
		logger.Error(ctx, "Error stopping server: %v", err)
	}

	logger.Info(ctx, "Server stopped")
}

// getLocalIP 获取本地 IP 地址
func getLocalIP() string {
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
