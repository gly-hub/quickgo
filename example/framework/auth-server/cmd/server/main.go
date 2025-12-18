package main

import (
	"fmt"

	"gly-hub/go-dandelion/quickgo"
	"gly-hub/go-dandelion/quickgo/db/gorm"
	"gly-hub/go-dandelion/quickgo/db/redis"
	gen "gly-hub/go-dandelion/quickgo/example/framework/auth-server/api/proto/gen/api/proto"
	"gly-hub/go-dandelion/quickgo/example/framework/auth-server/internal/handler"
	"gly-hub/go-dandelion/quickgo/example/framework/auth-server/internal/service"
	"gly-hub/go-dandelion/quickgo/tracing"

	rpc "google.golang.org/grpc"
	gormDB "gorm.io/gorm"
)

func main() {
	// 初始化配置（从配置文件加载）
	quickgo.InitConfig("local")

	// 加载配置到结构体
	var config = struct {
		AppConfig        quickgo.AppConfig        `json:"app" yaml:"app"`
		LoggerConfig     quickgo.LoggerConfig     `json:"logger" yaml:"logger"`
		GrpcServerConfig quickgo.GrpcServerConfig `json:"grpcServer" yaml:"grpcServer"`
		GormConfig       gorm.GormManagerConfig   `json:"gorm" yaml:"gorm"`
		RedisConfig      redis.RedisManagerConfig `json:"redis" yaml:"redis"`
		TracingConfig    tracing.Config           `json:"tracing" yaml:"tracing"`
	}{}
	quickgo.LoadCustomConfig(&config)

	// 创建框架实例，使用 Option 模式显式指定需要初始化的组件
	app, err := quickgo.NewFramework(
		quickgo.ConfigOptionWithApp(config.AppConfig),
		quickgo.ConfigOptionWithLogger(config.LoggerConfig),
		quickgo.ConfigOptionWithGrpcServer(&config.GrpcServerConfig),
		quickgo.ConfigOptionWithGorm(&config.GormConfig),
		quickgo.ConfigOptionWithRedis(&config.RedisConfig),
		quickgo.ConfigOptionWithTracing(&config.TracingConfig),
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
		// 获取数据库连接（如果配置了，必须成功获取，否则服务无法启动）
		var userDB *gormDB.DB
		var tokenCache *redis.Client

		// 如果配置了 GORM，必须成功获取连接
		if app.GormManager() != nil {
			db, err := app.GormManager().GetDB("go-admin")
			if err != nil {
				panic(fmt.Sprintf("failed to get GORM database connection 'go-admin' (service cannot start without database): %v", err))
			}
			userDB = db
		}

		// 如果配置了 Redis，必须成功获取连接
		if app.RedisManager() != nil {
			client, err := app.RedisManager().GetClient("token-cache")
			if err != nil {
				panic(fmt.Sprintf("failed to get Redis client 'token-cache' (service cannot start without Redis): %v", err))
			}
			tokenCache = client
		}

		// 创建认证服务（传入数据库连接）
		authService := service.NewAuthService(userDB, tokenCache)
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
