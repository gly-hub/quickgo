package main

import (
	"gly-hub/go-dandelion/quickgo"
	gen "gly-hub/go-dandelion/quickgo/example/framework/auth-server/api/proto/gen/api/proto"
	"gly-hub/go-dandelion/quickgo/example/framework/auth-server/internal/handler"
	"gly-hub/go-dandelion/quickgo/example/framework/auth-server/internal/service"

	rpc "google.golang.org/grpc"
)

func main() {
	// 初始化配置（从配置文件加载）
	quickgo.InitConfig("local")

	// 加载配置到结构体（使用 LoadCustomConfigKey 显式指定键名，推荐方式）
	var config = struct {
		AppConfig        quickgo.AppConfig        `json:"app" yaml:"app"`
		LoggerConfig     quickgo.LoggerConfig     `json:"logger" yaml:"logger"`
		GrpcServerConfig quickgo.GrpcServerConfig `json:"grpcServer" yaml:"grpcServer"`
	}{}
	quickgo.LoadCustomConfig(&config)

	// 创建框架实例，使用 Option 模式显式指定需要初始化的组件
	app, err := quickgo.NewFramework(
		quickgo.ConfigOptionWithApp(config.AppConfig),
		quickgo.ConfigOptionWithLogger(config.LoggerConfig),
		quickgo.ConfigOptionWithGrpcServer(&config.GrpcServerConfig),
		// 如果需要其他组件，可以继续添加：
		// quickgo.ConfigOptionWithGrpcClient(&grpcClientConfig),
		// quickgo.ConfigOptionWithHTTPServer(&httpServerConfig),
	)
	if err != nil {
		panic(err)
	}

	// 初始化所有组件
	if err := app.Init(); err != nil {
		panic(err)
	}

	// 注册 gRPC 服务
	if app.GrpcServer() != nil {
		// 创建认证服务
		authService := service.NewAuthService()
		// 创建认证处理器
		authHandler := handler.NewAuthHandler(authService)

		// 注册服务
		reg := func(s *rpc.Server) {
			gen.RegisterAuthServiceServer(s, authHandler)
		}
		app.GrpcServer().RegisterService(reg)
	}

	// 启动所有组件
	if err := app.Start(); err != nil {
		panic(err)
	}

	// 等待中断信号（优雅关闭）
	app.Wait()
}
