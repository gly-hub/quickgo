package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"quickgo/logger"
)

// ExampleUsage 使用示例
func ExampleUsage() {
	// 创建服务器配置
	config := Config{
		Address:    "0.0.0.0",
		Port:       50051,
		Reflection: true, // 启用反射，方便调试
		Options: []grpc.ServerOption{
			// 添加拦截器
			ChainUnaryInterceptors(
				LoggingInterceptor(),
				RecoveryInterceptor(),
			),
			// 添加keepalive配置
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    10,
				Timeout: 3,
			}),
		},
	}

	// 创建服务器实例
	ctx := context.Background()
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(ctx, "Failed to create gRPC server: %v", err)
	}

	// 注册服务示例
	// server.RegisterService(func(s *grpc.Server) {
	//     pb.RegisterYourServiceServer(s, &YourServiceImpl{})
	// })

	// 启动服务器（阻塞）
	// if err := server.Start(); err != nil {
	//     logger.Fatal(ctx, "Failed to start server: %v", err)
	// }

	// 或者异步启动
	// if err := server.StartAsync(); err != nil {
	//     logger.Fatal(ctx, "Failed to start server: %v", err)
	// }

	_ = server // 避免未使用变量警告
}

// ExampleWithTLS 使用TLS的示例
func ExampleWithTLS() {
	// 加载TLS证书
	ctx := context.Background()
	creds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
	if err != nil {
		logger.Fatal(ctx, "Failed to load TLS credentials: %v", err)
	}

	config := Config{
		Address: "0.0.0.0",
		Port:    50051,
		Options: []grpc.ServerOption{
			grpc.Creds(creds),
			ChainUnaryInterceptors(
				LoggingInterceptor(),
				RecoveryInterceptor(),
			),
		},
	}

	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(ctx, "Failed to create gRPC server: %v", err)
	}

	_ = server // 避免未使用变量警告

	// 注册服务并启动
	// ...
}

// ExampleGracefulShutdown 优雅关闭示例
func ExampleGracefulShutdown(ctx context.Context, server *Server) {
	// 启动服务器
	go func() {
		if err := server.Start(); err != nil {
			logger.Fatal(ctx, "Failed to start server: %v", err)
		}
	}()

	// 等待中断信号
	<-ctx.Done()

	// 优雅关闭
	if err := server.StopWithContext(ctx); err != nil {
		logger.Error(ctx, "Error stopping server: %v", err)
	}
}

// ExampleClientUsage 客户端使用示例
func ExampleClientUsage() {
	// 创建客户端配置
	config := ClientConfig{
		Address:  "localhost:50051",
		Timeout:  10 * time.Second,
		Insecure: true, // 使用非安全连接
		KeepAlive: &KeepAliveConfig{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		},
		Options: []grpc.DialOption{
			// 可以添加自定义拦截器
			ChainUnaryClientInterceptors(
				ClientAuthInterceptor("your-token"),
				ClientTimeoutInterceptor(5*time.Second),
			),
		},
	}

	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	// 连接到服务器
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect to server: %v", err)
	}

	// 健康检查
	healthResp, err := client.HealthCheck(ctx, "")
	if err != nil {
		logger.Error(ctx, "Health check failed: %v", err)
	} else {
		logger.Info(ctx, "Health check success: status=%s", healthResp.Status.String())
	}

	// 使用客户端连接调用服务
	// conn := client.GetConn()
	// serviceClient := pb.NewYourServiceClient(conn)
	// resp, err := serviceClient.YourMethod(ctx, &pb.YourRequest{...})
}

// ExampleClientWithTLS 使用TLS的客户端示例
func ExampleClientWithTLS() {
	// 创建带TLS的客户端配置
	config := ClientConfig{
		Address: "localhost:50051",
		Timeout: 10 * time.Second,
		TLS: &TLSConfig{
			CAFile:     "ca.crt",
			ServerName: "localhost",
		},
		KeepAlive: &KeepAliveConfig{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		},
	}

	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	// 连接到服务器
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect to server: %v", err)
	}

	// 使用客户端...
	_ = client
}

// ExampleClientWithClientCert 使用客户端证书的示例
// 注意：客户端证书需要使用自定义的 DialOption 来配置
func ExampleClientWithClientCert() {
	// 创建带TLS的配置（使用CA证书验证服务器）
	config := ClientConfig{
		Address: "localhost:50051",
		Timeout: 10 * time.Second,
		TLS: &TLSConfig{
			CAFile:     "ca.crt",
			ServerName: "localhost",
		},
		// 如果需要客户端证书，需要在 Options 中添加自定义的 DialOption
		// Options: []grpc.DialOption{
		//     grpc.WithTransportCredentials(/* 配置客户端证书的 credentials */),
		// },
	}

	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	// 连接到服务器
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect to server: %v", err)
	}

	// 使用客户端...
	_ = client
}

// ExampleClientWithTimeout 使用超时的客户端示例
func ExampleClientWithTimeout() {
	config := ClientConfig{
		Address:  "localhost:50051",
		Timeout:  5 * time.Second,
		Insecure: true,
	}

	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect to server: %v", err)
	}

	// 创建带超时的context
	timeoutCtx, cancel := client.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 使用超时context调用服务
	// serviceClient := pb.NewYourServiceClient(client.GetConn())
	// resp, err := serviceClient.YourMethod(timeoutCtx, &pb.YourRequest{...})
	_ = timeoutCtx
}

// ExampleServiceDiscovery 服务发现示例
func ExampleServiceDiscovery() {
	// 创建静态服务发现（指定多个服务地址）
	addresses := []string{
		"localhost:50051",
		"localhost:50052",
		"localhost:50053",
	}

	// 注册静态服务发现
	sd := NewStaticResolver(addresses)
	RegisterResolver(StaticScheme, sd)

	// 创建客户端配置（使用服务发现）
	config := ClientConfig{
		Address:          "my-service", // 服务名称
		Timeout:          10 * time.Second,
		Insecure:         true,
		ServiceDiscovery: sd,
		LoadBalancing:    PolicyRoundRobin, // 使用轮询负载均衡
		KeepAlive: &KeepAliveConfig{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		},
	}

	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	// 连接到服务器（会自动从服务发现获取地址并负载均衡）
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect to server: %v", err)
	}

	// 使用客户端...
	_ = client
}

// ExampleServiceRegistry 服务注册示例
func ExampleServiceRegistry() {
	// 创建服务注册中心（这里使用静态注册，实际可以使用 etcd、consul 等）
	registry := NewStaticRegistry()

	// 创建服务注册器
	serviceName := "my-service"
	address := "localhost:50051"
	metadata := map[string]string{
		"version": "1.0.0",
		"weight":  "10",
		"region":  "us-east-1",
	}

	registrar := NewServiceRegistrar(registry, serviceName, address, metadata)

	// 注册服务
	ctx := context.Background()
	if err := registrar.Register(ctx); err != nil {
		logger.Fatal(ctx, "Failed to register service: %v", err)
	}

	// 启动心跳（保持服务活跃）
	registrar.StartKeepAlive(30 * time.Second)

	// 服务关闭时注销
	defer func() {
		if err := registrar.Deregister(ctx); err != nil {
			logger.Error(ctx, "Failed to deregister service: %v", err)
		}
		registrar.Close()
	}()

	// 启动 gRPC 服务器...
	_ = registrar
}

// ExampleLoadBalancing 负载均衡示例
func ExampleLoadBalancing() {
	// 创建多个服务地址
	addresses := []string{
		"localhost:50051",
		"localhost:50052",
		"localhost:50053",
	}

	sd := NewStaticResolver(addresses)
	RegisterResolver(StaticScheme, sd)

	// 使用轮询负载均衡
	config := ClientConfig{
		Address:          "my-service",
		Timeout:          10 * time.Second,
		Insecure:         true,
		ServiceDiscovery: sd,
		LoadBalancing:    PolicyRoundRobin,
	}

	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect: %v", err)
	}

	// 多次调用会自动在多个服务实例间负载均衡
	// serviceClient := pb.NewYourServiceClient(client.GetConn())
	// for i := 0; i < 10; i++ {
	//     resp, err := serviceClient.YourMethod(ctx, &pb.YourRequest{...})
	//     // 请求会自动分发到不同的服务实例
	// }
	_ = client
}

// ExampleEtcdServiceDiscovery etcd 服务发现示例
func ExampleEtcdServiceDiscovery() {
	// 配置 etcd
	etcdConfig := EtcdConfig{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: 5 * time.Second,
		Prefix:      "/grpc/services",
	}

	// 创建 etcd resolver
	etcdResolver, err := NewEtcdResolver(etcdConfig)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create etcd resolver: %v", err)
	}
	defer etcdResolver.Close()

	// 注册 etcd resolver
	RegisterResolver(EtcdScheme, etcdResolver)

	// 创建客户端配置
	config := ClientConfig{
		Address:          "my-service", // 服务名称，会自动添加 etcd:// 前缀
		Timeout:          10 * time.Second,
		Insecure:         true,
		ServiceDiscovery: etcdResolver,
		LoadBalancing:    PolicyRoundRobin,
	}

	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create gRPC client: %v", err)
	}
	defer client.Close()

	// 连接到服务器（会自动从 etcd 获取服务地址）
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect to server: %v", err)
	}

	// 使用客户端...
	_ = client
}

// ExampleEtcdServiceRegistry etcd 服务注册示例
func ExampleEtcdServiceRegistry() {
	// 配置 etcd
	etcdConfig := EtcdConfig{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: 5 * time.Second,
		Prefix:      "/grpc/services",
		TTL:         30, // 30 秒 TTL
	}

	// 创建 etcd registry
	registry, err := NewEtcdRegistry(etcdConfig)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create etcd registry: %v", err)
	}
	defer registry.Close()

	// 创建服务注册器
	serviceName := "my-service"
	address := "localhost:50051"
	metadata := map[string]string{
		"version": "1.0.0",
		"weight":  "10",
		"region":  "us-east-1",
	}

	registrar := NewServiceRegistrar(registry, serviceName, address, metadata)

	// 注册服务
	ctx := context.Background()
	if err := registrar.Register(ctx); err != nil {
		logger.Fatal(ctx, "Failed to register service: %v", err)
	}

	// 启动心跳（保持服务活跃）
	registrar.StartKeepAlive(20 * time.Second) // 每 20 秒心跳一次

	// 服务关闭时注销
	defer func() {
		if err := registrar.Deregister(ctx); err != nil {
			logger.Error(ctx, "Failed to deregister service: %v", err)
		}
		registrar.Close()
	}()

	// 启动 gRPC 服务器...
	// server := NewServer(Config{...})
	// server.Start()
	_ = registrar
}

// ExampleEtcdWithAuth etcd 认证示例
func ExampleEtcdWithAuth() {
	// 配置 etcd（带认证）
	etcdConfig := EtcdConfig{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: 5 * time.Second,
		Prefix:      "/grpc/services",
		Username:    "root",
		Password:    "password",
	}

	// 创建 etcd resolver
	etcdResolver, err := NewEtcdResolver(etcdConfig)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create etcd resolver: %v", err)
	}
	defer etcdResolver.Close()

	// 使用 etcd resolver...
	_ = etcdResolver
}
