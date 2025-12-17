package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gly-hub/go-dandelion/quickgo/http"
	"gly-hub/go-dandelion/quickgo/logger"

	"github.com/gofiber/fiber/v2"
)

func main() {
	ctx := context.Background()

	// 初始化 logger
	logger.Init(logger.Config{
		Level: logger.LevelDebug,
	})

	// 创建服务器配置
	config := http.Config{
		Address: "0.0.0.0",
		Port:    9999,
		// 启用默认中间件
		EnableCORS:     true,
		EnableRecovery: true,
		EnableLogging:  true,
		EnableTrace:    true,
		// TraceMiddleware 已经同时设置了 trace_id 和 request_id
	}

	// 创建服务器
	server, err := http.NewServer(config)
	if err != nil {
		logger.Fatal(ctx, "Failed to create server: %v", err)
	}

	// 获取 Fiber 应用实例
	app := server.GetApp()

	// 注册路由
	setupRoutes(app)

	// 启动服务器（异步）
	go func() {
		logger.Info(ctx, "HTTP server starting on %s:%d", config.Address, config.Port)
		if err := server.Start(); err != nil {
			logger.Fatal(ctx, "Failed to start server: %v", err)
		}
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info(ctx, "Shutting down server...")
	if err := server.Stop(); err != nil {
		logger.Error(ctx, "Error stopping server: %v", err)
	}

	logger.Info(ctx, "Server stopped")
}

// setupRoutes 设置路由
func setupRoutes(app *fiber.App) {
	// 根路由
	app.Get("/", handleHome)

	// 健康检查
	app.Get("/health", handleHealth)

	// API 路由组
	api := app.Group("/api")
	{
		// 用户相关路由
		api.Get("/users", handleGetUsers)
		api.Get("/users/:id", handleGetUser)
		api.Post("/users", handleCreateUser)

		// 示例路由：展示链路追踪
		api.Get("/trace", handleTrace)
	}
}

// handleHome 处理首页
func handleHome(c *fiber.Ctx) error {
	traceID := http.GetTraceID(c)
	requestID := http.GetRequestID(c)

	// 从 Locals 中获取 trace ID，创建 context
	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	logger.Info(ctx, "Home page accessed")

	return c.JSON(fiber.Map{
		"message":    "Welcome to QuickGo HTTP Server",
		"trace_id":   traceID,
		"request_id": requestID,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
}

// handleHealth 处理健康检查
func handleHealth(c *fiber.Ctx) error {
	traceID := http.GetTraceID(c)

	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	logger.Info(ctx, "Health check")

	return c.JSON(fiber.Map{
		"status":   "ok",
		"service":  "quickgo-http",
		"trace_id": traceID,
	})
}

// handleGetUsers 获取用户列表
func handleGetUsers(c *fiber.Ctx) error {
	traceID := http.GetTraceID(c)

	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	logger.Info(ctx, "Getting users list")

	// 模拟用户数据
	users := []fiber.Map{
		{
			"id":    1,
			"name":  "Alice",
			"email": "alice@example.com",
		},
		{
			"id":    2,
			"name":  "Bob",
			"email": "bob@example.com",
		},
		{
			"id":    3,
			"name":  "Charlie",
			"email": "charlie@example.com",
		},
	}

	return c.JSON(fiber.Map{
		"users":    users,
		"count":    len(users),
		"trace_id": traceID,
	})
}

// handleGetUser 获取单个用户
func handleGetUser(c *fiber.Ctx) error {
	traceID := http.GetTraceID(c)
	userID := c.Params("id")

	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	logger.Info(ctx, "Getting user: id=%s", userID)

	// 模拟查找用户
	user := fiber.Map{
		"id":    userID,
		"name":  "User " + userID,
		"email": "user" + userID + "@example.com",
	}

	return c.JSON(fiber.Map{
		"user":     user,
		"trace_id": traceID,
	})
}

// handleCreateUser 创建用户
func handleCreateUser(c *fiber.Ctx) error {
	traceID := http.GetTraceID(c)

	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	// 解析请求体
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error(ctx, "Failed to parse request body: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":    "Invalid request body",
			"trace_id": traceID,
		})
	}

	logger.Info(ctx, "Creating user: name=%s, email=%s", req.Name, req.Email)

	// 模拟创建用户
	user := fiber.Map{
		"id":    4,
		"name":  req.Name,
		"email": req.Email,
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":  "User created successfully",
		"user":     user,
		"trace_id": traceID,
	})
}

// handleTrace 展示链路追踪信息
func handleTrace(c *fiber.Ctx) error {
	traceID := http.GetTraceID(c)
	spanID := http.GetSpanID(c)
	requestID := http.GetRequestID(c)

	ctx := context.Background()
	if traceID != "" {
		ctx = logger.WithTrace(ctx, traceID, spanID)
	} else {
		ctx = logger.StartSpan(ctx)
	}

	logger.Info(ctx, "Trace information requested")
	logger.Debug(ctx, "Processing trace request")

	return c.JSON(fiber.Map{
		"trace_id":   traceID,
		"span_id":    spanID,
		"request_id": requestID, // request_id 和 trace_id 是同一个值
		"message":    "This endpoint demonstrates trace ID propagation",
		"note":       "request_id and trace_id use the same value, unified header: X-Trace-ID",
		"headers": fiber.Map{
			"x-trace-id": c.Get("X-Trace-ID"),
		},
	})
}
