package http

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"quickgo/logger"
)

// ExampleBasicServer 基础服务器示例
func ExampleBasicServer() {
	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
	})

	// 创建服务器配置
	config := Config{
		Address: "0.0.0.0",
		Port:    8080,
		// 启用默认中间件
		EnableCORS:     true,
		EnableRecovery: true,
		EnableLogging:  true,
		EnableTrace:    true,
	}

	// 创建服务器
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create server: %v", err)
	}

	// 注册路由
	app := server.GetApp()
	app.Get("/", func(c *fiber.Ctx) error {
		// 从 Locals 中获取 trace ID，创建 context
		traceID := GetTraceID(c)
		ctx := context.Background()
		if traceID != "" {
			ctx = logger.WithTraceID(ctx, traceID)
		} else {
			ctx = logger.StartSpan(ctx)
		}
		logger.Info(ctx, "Handling request")
		return c.JSON(fiber.Map{
			"message":  "Hello, World!",
			"trace_id": GetTraceID(c),
		})
	})

	// 启动服务器
	if err := server.Start(); err != nil {
		logger.Fatal(context.Background(), "Failed to start server: %v", err)
	}
}

// ExampleWithCustomMiddleware 自定义中间件示例
func ExampleWithCustomMiddleware() {
	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
	})

	// 创建自定义中间件
	customMiddleware := func(c *fiber.Ctx) error {
		// 从 Locals 中获取 trace ID，创建 context
		traceID := GetTraceID(c)
		ctx := context.Background()
		if traceID != "" {
			ctx = logger.WithTraceID(ctx, traceID)
		} else {
			ctx = logger.StartSpan(ctx)
		}
		logger.Info(ctx, "Custom middleware executed")
		return c.Next()
	}

	// 创建服务器配置
	config := Config{
		Address: "0.0.0.0",
		Port:    8080,
		Middlewares: []fiber.Handler{
			customMiddleware,
		},
	}

	// 创建服务器
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create server: %v", err)
	}

	// 注册路由
	app := server.GetApp()
	app.Get("/api/users", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"users": []string{"user1", "user2"},
		})
	})

	// 启动服务器
	if err := server.Start(); err != nil {
		logger.Fatal(context.Background(), "Failed to start server: %v", err)
	}
}

// ExampleWithCORS  CORS 配置示例
func ExampleWithCORS() {
	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
	})

	// 创建服务器配置
	config := Config{
		Address:    "0.0.0.0",
		Port:       8080,
		EnableCORS: true,
		CORSConfig: CORSConfig{
			AllowOrigins:     "https://example.com,https://www.example.com",
			AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
			AllowHeaders:     "Content-Type,Authorization",
			AllowCredentials: true,
			MaxAge:           3600,
		},
	}

	// 创建服务器
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create server: %v", err)
	}

	// 注册路由
	app := server.GetApp()
	app.Get("/api/data", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"data": "protected data",
		})
	})

	// 启动服务器
	if err := server.Start(); err != nil {
		logger.Fatal(context.Background(), "Failed to start server: %v", err)
	}
}

// ExampleWithTimeout 超时中间件示例
func ExampleWithTimeout() {
	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
	})

	// 创建服务器配置
	config := Config{
		Address: "0.0.0.0",
		Port:    8080,
		Middlewares: []fiber.Handler{
			TimeoutMiddleware(5 * time.Second),
		},
	}

	// 创建服务器
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create server: %v", err)
	}

	// 注册路由
	app := server.GetApp()
	app.Get("/api/slow", func(c *fiber.Ctx) error {
		// 模拟慢请求
		time.Sleep(10 * time.Second)
		return c.JSON(fiber.Map{
			"message": "This should timeout",
		})
	})

	// 启动服务器
	if err := server.Start(); err != nil {
		logger.Fatal(context.Background(), "Failed to start server: %v", err)
	}
}

// ExampleWithRequestID 请求 ID 中间件示例
func ExampleWithRequestID() {
	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
	})

	// 创建服务器配置
	config := Config{
		Address: "0.0.0.0",
		Port:    8080,
		Middlewares: []fiber.Handler{
			RequestIDMiddleware(),
		},
	}

	// 创建服务器
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to create server: %v", err)
	}

	// 注册路由
	app := server.GetApp()
	app.Get("/api/request", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"request_id": GetRequestID(c),
			"trace_id":   GetTraceID(c),
		})
	})

	// 启动服务器
	if err := server.Start(); err != nil {
		logger.Fatal(context.Background(), "Failed to start server: %v", err)
	}
}
