package quickgo

import (
	"context"
	"quickgo/logger"

	rpc "google.golang.org/grpc"

	"github.com/gofiber/fiber/v2"
)

// ExampleBasicFramework 基础框架使用示例
func ExampleBasicFramework() {
	// 1. 初始化配置（从配置文件加载）
	InitConfig("local")

	// 2. 加载配置到结构体
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	var grpcServerConfig GrpcServerConfig
	var httpServerConfig HTTPServerConfig
	LoadCustomConfig(&appConfig, &loggerConfig, &grpcServerConfig, &httpServerConfig)

	// 3. 创建框架实例，使用 Option 模式显式指定需要初始化的组件
	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		ConfigOptionWithGrpcServer(&grpcServerConfig),
		ConfigOptionWithHTTPServer(&httpServerConfig),
		// 不需要的组件直接注释掉即可
	)
	if err != nil {
		panic(err)
	}

	// 4. 初始化所有组件
	if err := app.Init(); err != nil {
		panic(err)
	}

	// 5. 注册 gRPC 服务（如果启用了 gRPC Server）
	if app.GrpcServer() != nil {
		// 使用 RegisterService 注册服务
		app.GrpcServer().RegisterService(func(s *rpc.Server) {
			// 注册你的 gRPC 服务
			// 注意：这里的 grpc.Server 是 google.golang.org/grpc 包的类型
			// pb.RegisterYourServiceServer(s, &YourService{})
		})
	}

	// 6. 注册 HTTP 路由（如果启用了 HTTP Server）
	if app.HTTPServer() != nil {
		app.HTTPServer().GetApp().Get("/", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Hello, World!"})
		})
	}

	// 7. 启动所有组件
	if err := app.Start(); err != nil {
		panic(err)
	}

	// 8. 等待中断信号（优雅关闭）
	app.Wait()
}

// ExampleWithCustomConfig 使用自定义配置的示例
func ExampleWithCustomConfig() {
	// 创建自定义配置
	appConfig := AppConfig{
		Name:    "my-service",
		Version: "1.0.0",
		Env:     "local",
	}

	loggerConfig := LoggerConfig{
		Enabled: true,
		Level:   "debug",
		Output:  "console",
		Service: "my-service",
		Version: "1.0.0",
	}

	grpcServerConfig := GrpcServerConfig{
		ServiceName:      "my-service",
		Address:          "0.0.0.0",
		Port:             50051,
		KeepAliveTime:    "10s",
		KeepAliveTimeout: "3s",
		Etcd: &EtcdConfig{
			Endpoints:   []string{"127.0.0.1:2379"},
			DialTimeout: "5s",
			Prefix:      "/grpc/services",
			TTL:         30,
		},
	}

	httpServerConfig := HTTPServerConfig{
		Enabled:        true,
		Address:        "0.0.0.0",
		Port:           8080,
		EnableCORS:     true,
		EnableRecovery: true,
		EnableLogging:  true,
		EnableTrace:    true,
	}

	// 创建框架，使用 Option 模式
	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		ConfigOptionWithGrpcServer(&grpcServerConfig),
		ConfigOptionWithHTTPServer(&httpServerConfig),
	)
	if err != nil {
		panic(err)
	}

	app.Init()
	app.Start()
	app.Wait()
}

// ExampleGatewayService 网关服务示例（使用 gRPC Client Manager）
func ExampleGatewayService() {
	// 创建配置
	appConfig := AppConfig{
		Name:    "gateway",
		Version: "1.0.0",
		Env:     "local",
	}

	loggerConfig := LoggerConfig{
		Enabled: true,
		Level:   "info",
		Output:  "console",
		Service: "gateway",
		Version: "1.0.0",
	}

	grpcClientConfig := GrpcClientConfig{
		Timeout:       "10s",
		Insecure:      true,
		LoadBalancing: "round_robin",
		Etcd: &EtcdConfig{
			Endpoints:   []string{"127.0.0.1:2379"},
			DialTimeout: "5s",
			Prefix:      "/grpc/services",
		},
	}

	httpServerConfig := HTTPServerConfig{
		Enabled:        true,
		Address:        "0.0.0.0",
		Port:           8080,
		EnableCORS:     true,
		EnableRecovery: true,
		EnableLogging:  true,
		EnableTrace:    true,
	}

	// 创建框架，使用 Option 模式
	// 注意：网关服务不需要 gRPC Server，所以不添加 ConfigOptionWithGrpcServer
	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		ConfigOptionWithGrpcClient(&grpcClientConfig),
		ConfigOptionWithHTTPServer(&httpServerConfig),
	)
	if err != nil {
		panic(err)
	}

	app.Init()

	// 注册需要调用的 gRPC 服务
	clientMgr := app.GrpcClientManager()
	if clientMgr != nil {
		clientMgr.RegisterService("user-service")
		clientMgr.RegisterService("order-service")
	}

	// 在 HTTP 路由中使用 gRPC 客户端
	httpServer := app.HTTPServer()
	if httpServer != nil {
		httpServer.GetApp().Get("/api/users/:id", func(c *fiber.Ctx) error {
			// 获取用户服务连接
			_, err := clientMgr.GetConn(c.Context(), "user-service")
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}

			// 创建服务客户端并调用
			// conn, _ := clientMgr.GetConn(c.Context(), "user-service")
			// userClient := pb.NewUserServiceClient(conn)
			// resp, err := userClient.GetUser(c.Context(), &pb.GetUserRequest{Id: c.Params("id")})
			// ...

			return c.JSON(fiber.Map{"message": "success"})
		})
	}

	app.Start()
	app.Wait()
}

// ExampleWithCustomComponent 使用自定义组件的示例
func ExampleWithCustomComponent() {
	// 创建框架，只配置必要的组件（App 和 Logger）
	appConfig := AppConfig{
		Name:    "custom-service",
		Version: "1.0.0",
		Env:     "local",
	}

	loggerConfig := LoggerConfig{
		Enabled: true,
		Level:   "info",
		Output:  "console",
		Service: "custom-service",
		Version: "1.0.0",
	}

	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		// 不添加其他组件，只使用自定义组件
	)
	if err != nil {
		panic(err)
	}

	// 注册自定义组件
	app.RegisterComponent(&MyCustomComponent{
		enabled: true,
	})

	app.Init()
	app.Start()
	app.Wait()
}

// MyCustomComponent 自定义组件示例
type MyCustomComponent struct {
	enabled bool
}

func (c *MyCustomComponent) Name() string {
	return "my-custom-component"
}

func (c *MyCustomComponent) Init(ctx context.Context) error {
	logger.Info(ctx, "Initializing custom component")
	return nil
}

func (c *MyCustomComponent) Start(ctx context.Context) error {
	logger.Info(ctx, "Starting custom component")
	return nil
}

func (c *MyCustomComponent) Stop(ctx context.Context) error {
	logger.Info(ctx, "Stopping custom component")
	return nil
}

func (c *MyCustomComponent) IsEnabled() bool {
	return c.enabled
}
