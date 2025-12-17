package main

import (
	"context"
	"fmt"
	hello2 "gly-hub/go-dandelion/quickgo/example/grpc/proto/gen/proto"
	"io"
	"os"
	"time"

	"gly-hub/go-dandelion/quickgo/grpc"
	"gly-hub/go-dandelion/quickgo/logger"
)

const (
	// 服务配置
	serviceName = "hello-service"
	// etcd 配置
	defaultEtcdEndpoint = "127.0.0.1:2379"
	etcdPrefix          = "/grpc/services"
)

func main() {
	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
		//Output: logger.OutputConsole,
	})

	// 生成 trace ID 并设置到 context 中
	ctx := context.Background()
	traceID := logger.GenerateTraceID()
	ctx = logger.WithTraceID(ctx, traceID)
	logger.Info(ctx, "Client started with trace ID: %s", traceID)

	// 获取 etcd 端点（可以从环境变量读取）
	etcdEndpoint := os.Getenv("ETCD_ENDPOINT")
	if etcdEndpoint == "" {
		etcdEndpoint = defaultEtcdEndpoint
	}

	// 创建 etcd resolver 配置
	etcdConfig := grpc.EtcdConfig{
		Endpoints:   []string{etcdEndpoint},
		DialTimeout: 5 * time.Second,
		Prefix:      etcdPrefix,
	}

	// 创建 etcd resolver
	etcdResolver, err := grpc.NewEtcdResolver(etcdConfig)
	if err != nil {
		logger.Fatal(ctx, "Failed to create etcd resolver: %v", err)
	}
	defer etcdResolver.Close()

	// 注册 etcd resolver
	grpc.RegisterResolver(grpc.EtcdScheme, etcdResolver)

	// 创建客户端配置（使用服务名称，etcd 会自动解析）
	config := grpc.ClientConfig{
		Address:          serviceName, // 使用服务名称，etcd 会自动解析为实际地址
		Timeout:          10 * time.Second,
		Insecure:         true,
		ServiceDiscovery: etcdResolver,
		LoadBalancing:    grpc.PolicyRoundRobin, // 使用轮询负载均衡
	}

	// 创建客户端
	client, err := grpc.NewClient(config)
	if err != nil {
		logger.Fatal(ctx, "Failed to create client: %v", err)
	}
	defer client.Close()

	// 连接到服务器（从 etcd 获取服务地址）
	if err := client.Connect(ctx); err != nil {
		logger.Fatal(ctx, "Failed to connect: %v", err)
	}

	logger.Info(ctx, "Connected to server successfully via etcd service discovery")

	// 创建服务客户端
	helloClient := hello2.NewHelloServiceClient(client.GetConn())

	// 测试 1: 简单 RPC 调用
	testSayHello(ctx, helloClient)

	// 测试 2: 服务端流式调用
	testSayHelloStream(ctx, helloClient)

	// 测试 3: 客户端流式调用
	testSayHelloClientStream(ctx, helloClient)

	// 测试 4: 双向流式调用
	testSayHelloBidiStream(ctx, helloClient)
}

// testSayHello 测试简单 RPC 调用
func testSayHello(ctx context.Context, client hello2.HelloServiceClient) {
	logger.Info(ctx, "=== Testing SayHello ===")

	req := &hello2.HelloRequest{
		Name:    "Alice",
		Message: "Hello from client!",
	}

	resp, err := client.SayHello(ctx, req)
	if err != nil {
		logger.Error(ctx, "SayHello failed: %v", err)
		return
	}

	logger.Info(ctx, "SayHello Response: greeting=%s, code=%d, message=%s",
		resp.Greeting, resp.Code, resp.Message)
	fmt.Printf("Response: %s\n", resp.Greeting)
}

// testSayHelloStream 测试服务端流式调用
func testSayHelloStream(ctx context.Context, client hello2.HelloServiceClient) {
	logger.Info(ctx, "=== Testing SayHelloStream ===")

	req := &hello2.HelloRequest{
		Name:    "Bob",
		Message: "Stream test",
	}

	stream, err := client.SayHelloStream(ctx, req)
	if err != nil {
		logger.Error(ctx, "SayHelloStream failed: %v", err)
		return
	}

	// 接收流式响应
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			logger.Info(ctx, "Stream ended")
			break
		}
		if err != nil {
			logger.Error(ctx, "Stream receive error: %v", err)
			return
		}

		logger.Info(ctx, "Stream Response: greeting=%s, message=%s",
			resp.Greeting, resp.Message)
		fmt.Printf("Stream: %s\n", resp.Greeting)
	}
}

// testSayHelloClientStream 测试客户端流式调用
func testSayHelloClientStream(ctx context.Context, client hello2.HelloServiceClient) {
	logger.Info(ctx, "=== Testing SayHelloClientStream ===")

	stream, err := client.SayHelloClientStream(ctx)
	if err != nil {
		logger.Error(ctx, "SayHelloClientStream failed: %v", err)
		return
	}

	// 发送多个请求
	names := []string{"Charlie", "David", "Eve"}
	for i, name := range names {
		req := &hello2.HelloRequest{
			Name:    name,
			Message: fmt.Sprintf("Message #%d", i+1),
		}

		if err := stream.Send(req); err != nil {
			logger.Error(ctx, "Stream send error: %v", err)
			return
		}

		logger.Info(ctx, "Sent client stream message: name=%s", name)
		time.Sleep(200 * time.Millisecond)
	}

	// 关闭发送并接收响应
	resp, err := stream.CloseAndRecv()
	if err != nil {
		logger.Error(ctx, "CloseAndRecv failed: %v", err)
		return
	}

	logger.Info(ctx, "Client Stream Response: greeting=%s, code=%d",
		resp.Greeting, resp.Code)
	fmt.Printf("Response: %s\n", resp.Greeting)
}

// testSayHelloBidiStream 测试双向流式调用
func testSayHelloBidiStream(ctx context.Context, client hello2.HelloServiceClient) {
	logger.Info(ctx, "=== Testing SayHelloBidiStream ===")

	stream, err := client.SayHelloBidiStream(ctx)
	if err != nil {
		logger.Error(ctx, "SayHelloBidiStream failed: %v", err)
		return
	}

	// 启动 goroutine 接收响应
	done := make(chan bool)
	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				done <- true
				return
			}
			if err != nil {
				logger.Error(ctx, "Bidi stream receive error: %v", err)
				done <- true
				return
			}

			logger.Info(ctx, "Bidi Stream Response: greeting=%s", resp.Greeting)
			fmt.Printf("Echo: %s\n", resp.Greeting)
		}
	}()

	// 发送多个请求
	names := []string{"Frank", "Grace", "Henry"}
	for i, name := range names {
		req := &hello2.HelloRequest{
			Name:    name,
			Message: fmt.Sprintf("Bidi message #%d", i+1),
		}

		if err := stream.Send(req); err != nil {
			logger.Error(ctx, "Bidi stream send error: %v", err)
			break
		}

		logger.Info(ctx, "Sent bidi stream message: name=%s", name)
		time.Sleep(300 * time.Millisecond)
	}

	// 关闭发送
	if err := stream.CloseSend(); err != nil {
		logger.Error(ctx, "CloseSend failed: %v", err)
	}

	// 等待接收完成
	<-done
	logger.Info(ctx, "Bidi stream completed")
}
