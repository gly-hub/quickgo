package main

import (
	"context"
	"strconv"
	"time"

	"github.com/gly-hub/quickgo"
	gen "github.com/gly-hub/quickgo/example/simple/proto/gen"
	"github.com/gly-hub/quickgo/grpcep"
	"github.com/gly-hub/quickgo/logger"
	"github.com/gly-hub/quickgo/tracing"

	"github.com/gofiber/fiber/v2"
)

// UserHandler 用户处理器
type UserHandler struct {
	clientMgr *quickgo.GrpcClientManager
}

// NewUserHandler 创建用户处理器
func NewUserHandler(clientMgr *quickgo.GrpcClientManager) *UserHandler {
	return &UserHandler{clientMgr: clientMgr}
}

// GetUser 获取用户信息
func (h *UserHandler) GetUser(c *fiber.Ctx) error {
	ctx := c.UserContext()

	idStr := c.Params("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.Status(400).JSON(grpcep.JsonResponse{
			Code: grpcep.ParamsErrCode,
			Msg:  "invalid user id",
		})
	}

	// 获取 gRPC 连接
	conn, err := h.clientMgr.GetConn(ctx, "user-service")
	if err != nil {
		logger.Error(ctx, "Failed to get gRPC connection: %v", err)
		return c.Status(500).JSON(grpcep.JsonResponse{
			Code: grpcep.InternalErrCode,
			Msg:  "service unavailable",
		})
	}

	// 调用 RPC
	client := gen.NewUserServiceClient(conn)
	resp, err := client.GetUser(ctx, &gen.GetUserRequest{Id: id})
	if err != nil {
		logger.Error(ctx, "RPC GetUser failed: %v", err)
		return c.Status(500).JSON(grpcep.JsonResponse{
			Code: grpcep.InternalErrCode,
			Msg:  "rpc call failed",
		})
	}

	return c.JSON(grpcep.JsonResponse{
		Code: resp.CommonResp.Code,
		Msg:  resp.CommonResp.Msg,
		Data: resp.User,
	})
}

// CreateUser 创建用户
func (h *UserHandler) CreateUser(c *fiber.Ctx) error {
	ctx := c.UserContext()

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(grpcep.JsonResponse{
			Code: grpcep.ParamsErrCode,
			Msg:  "invalid request body",
		})
	}

	// 获取 gRPC 连接
	conn, err := h.clientMgr.GetConn(ctx, "user-service")
	if err != nil {
		logger.Error(ctx, "Failed to get gRPC connection: %v", err)
		return c.Status(500).JSON(grpcep.JsonResponse{
			Code: grpcep.InternalErrCode,
			Msg:  "service unavailable",
		})
	}

	// 调用 RPC
	client := gen.NewUserServiceClient(conn)
	resp, err := client.CreateUser(ctx, &gen.CreateUserRequest{
		Username: req.Username,
		Email:    req.Email,
		Phone:    req.Phone,
	})
	if err != nil {
		logger.Error(ctx, "RPC CreateUser failed: %v", err)
		return c.Status(500).JSON(grpcep.JsonResponse{
			Code: grpcep.InternalErrCode,
			Msg:  "rpc call failed",
		})
	}

	return c.JSON(grpcep.JsonResponse{
		Code: resp.CommonResp.Code,
		Msg:  resp.CommonResp.Msg,
		Data: resp.User,
	})
}

// ListUsers 用户列表
func (h *UserHandler) ListUsers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "10"))

	// 获取 gRPC 连接
	conn, err := h.clientMgr.GetConn(ctx, "user-service")
	if err != nil {
		logger.Error(ctx, "Failed to get gRPC connection: %v", err)
		return c.Status(500).JSON(grpcep.JsonResponse{
			Code: grpcep.InternalErrCode,
			Msg:  "service unavailable",
		})
	}

	// 调用 RPC
	client := gen.NewUserServiceClient(conn)
	resp, err := client.ListUsers(ctx, &gen.ListUsersRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		logger.Error(ctx, "RPC ListUsers failed: %v", err)
		return c.Status(500).JSON(grpcep.JsonResponse{
			Code: grpcep.InternalErrCode,
			Msg:  "rpc call failed",
		})
	}

	return c.JSON(grpcep.JsonResponse{
		Code: resp.CommonResp.Code,
		Msg:  resp.CommonResp.Msg,
		Data: fiber.Map{
			"users": resp.Users,
			"total": resp.Total,
			"page":  page,
		},
	})
}

// HealthCheck 健康检查
func (h *UserHandler) HealthCheck(c *fiber.Ctx) error {
	ctx := c.UserContext()

	result := fiber.Map{
		"status":    "healthy",
		"service":   "gateway",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// 检查上游服务健康状态
	conn, err := h.clientMgr.GetConn(ctx, "user-service")
	if err != nil {
		result["upstream"] = fiber.Map{
			"user-service": "unavailable",
		}
	} else {
		client := gen.NewUserServiceClient(conn)
		resp, err := client.HealthCheck(ctx, &gen.HealthCheckRequest{})
		if err != nil {
			result["upstream"] = fiber.Map{
				"user-service": "unhealthy",
			}
		} else {
			result["upstream"] = fiber.Map{
				"user-service": resp.Status,
				"version":      resp.Version,
			}
		}
	}

	return c.JSON(result)
}

func main() {
	// 初始化配置（配置文件在上级目录的 config 文件夹）
	quickgo.InitConfig("local", "../config")

	// 加载配置
	var config struct {
		App        quickgo.AppConfig        `yaml:"app"`
		Logger     quickgo.LoggerConfig     `yaml:"logger"`
		GrpcClient quickgo.GrpcClientConfig `yaml:"grpcClient"`
		HttpServer quickgo.HTTPServerConfig `yaml:"httpServer"`
		Tracing    tracing.Config           `yaml:"tracing"`
	}
	quickgo.LoadCustomConfig(&config)

	// 创建框架实例
	app, err := quickgo.NewFramework(
		quickgo.ConfigOptionWithApp(config.App),
		quickgo.ConfigOptionWithLogger(config.Logger),
		quickgo.ConfigOptionWithGrpcClient(&config.GrpcClient),
		quickgo.ConfigOptionWithHTTPServer(&config.HttpServer),
		quickgo.ConfigOptionWithTracing(&config.Tracing),
	)
	if err != nil {
		panic(err)
	}

	// 初始化
	if err := app.Init(); err != nil {
		panic(err)
	}

	// 注册上游服务
	app.GrpcClientManager().RegisterService("user-service")

	// 创建处理器
	userHandler := NewUserHandler(app.GrpcClientManager())

	// 注册路由
	app.HTTPServer().RegisterApp(func(fiberApp *fiber.App) {
		// 健康检查
		fiberApp.Get("/health", userHandler.HealthCheck)

		// API 路由
		api := fiberApp.Group("/api/v1")
		{
			users := api.Group("/users")
			{
				users.Get("/", userHandler.ListUsers)
				users.Get("/:id", userHandler.GetUser)
				users.Post("/", userHandler.CreateUser)
			}
		}
	})

	// 启动
	if err := app.Start(); err != nil {
		panic(err)
	}

	logger.Info(context.Background(), "Gateway started successfully")

	// 等待退出
	app.Wait()
}
