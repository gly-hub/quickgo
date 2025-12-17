package main

import (
	"context"
	"gly-hub/go-dandelion/quickgo"
	"gly-hub/go-dandelion/quickgo/example/framework/gateway/internal/handler"
	"gly-hub/go-dandelion/quickgo/logger"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

// grpcClientManagerAdapter 适配器，将 GrpcClientManager 适配为 handler.ClientManager 接口
type grpcClientManagerAdapter struct {
	manager *quickgo.GrpcClientManager
}

func (a *grpcClientManagerAdapter) GetConn(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	return a.manager.GetConn(ctx, serviceName)
}

func main() {
	// 初始化配置（从配置文件加载）
	quickgo.InitConfig("local")

	// 加载配置到结构体
	var config = struct {
		AppConfig        *quickgo.AppConfig        `json:"app" yaml:"app"`
		LoggerConfig     *quickgo.LoggerConfig     `json:"logger" yaml:"logger"`
		GrpcClientConfig *quickgo.GrpcClientConfig `json:"grpcClient" yaml:"grpcClient"`
		HttpServerConfig *quickgo.HTTPServerConfig `json:"httpServer" yaml:"httpServer"`
	}{}
	quickgo.LoadCustomConfig(&config)

	// 创建框架实例，使用 Option 模式显式指定需要初始化的组件
	app, err := quickgo.NewFramework(
		quickgo.ConfigOptionWithApp(*config.AppConfig),
		quickgo.ConfigOptionWithLogger(*config.LoggerConfig),
		quickgo.ConfigOptionWithGrpcClient(config.GrpcClientConfig),
		quickgo.ConfigOptionWithHTTPServer(config.HttpServerConfig),
		// 如果不需要某个组件，直接注释掉即可，例如：
		// quickgo.ConfigOptionWithGrpcServer(&grpcServerConfig),
	)
	if err != nil {
		panic(err)
	}

	// 初始化所有组件
	if err := app.Init(); err != nil {
		panic(err)
	}

	// 注册需要调用的 gRPC 服务
	if app.GrpcClientManager() != nil {
		app.GrpcClientManager().RegisterService("auth-service")
	}

	// 注册 HTTP 路由
	if app.HTTPServer() != nil {
		// 创建认证处理器（需要实现 ClientManager 接口的适配器）
		clientMgr := &grpcClientManagerAdapter{manager: app.GrpcClientManager()}
		authHandler := handler.NewAuthHandler(clientMgr)

		// 注册路由
		app.HTTPServer().RegisterApp(func(fiberApp *fiber.App) {
			// 健康检查
			fiberApp.Get("/health", func(c *fiber.Ctx) error {
				ctx := context.Background()
				traceID := c.Get("X-Trace-ID")
				if traceID != "" {
					ctx = logger.WithTraceID(ctx, traceID)
				}
				logger.Info(ctx, "Health check")
				return c.JSON(fiber.Map{
					"status":  "ok",
					"service": "gateway",
				})
			})

			// API 路由组
			api := fiberApp.Group("/api/v1")
			{
				// 认证相关路由
				auth := api.Group("/auth")
				{
					auth.Post("/login", authHandler.Login)
					auth.Get("/verify", authHandler.VerifyToken)
					auth.Post("/refresh", authHandler.RefreshToken)
					auth.Get("/user/:id", authHandler.GetUserInfo)
				}
			}
		})
	}

	// 启动所有组件
	if err := app.Start(); err != nil {
		panic(err)
	}

	// 等待中断信号（优雅关闭）
	app.Wait()
}
