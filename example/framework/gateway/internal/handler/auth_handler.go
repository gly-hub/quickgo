package handler

import (
	"context"
	"gly-hub/go-dandelion/quickgo/example/framework/gateway/internal/service"
	"gly-hub/go-dandelion/quickgo/logger"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

// AuthHandler HTTP 认证处理器
type AuthHandler struct {
	authClient *service.AuthClient
	clientMgr  ClientManager
}

// ClientManager gRPC 客户端管理器接口
type ClientManager interface {
	GetConn(ctx context.Context, serviceName string) (*grpc.ClientConn, error)
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(clientMgr ClientManager) *AuthHandler {
	return &AuthHandler{
		clientMgr: clientMgr,
	}
}

// getAuthClient 获取认证客户端
func (h *AuthHandler) getAuthClient(ctx context.Context) (*service.AuthClient, error) {
	if h.authClient != nil {
		return h.authClient, nil
	}

	// 获取 gRPC 连接
	conn, err := h.clientMgr.GetConn(ctx, "auth-service")
	if err != nil {
		return nil, err
	}

	// 创建客户端
	h.authClient = service.NewAuthClient(conn)
	return h.authClient, nil
}

// Login 用户登录
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	ctx := context.Background()

	// 获取 trace ID
	traceID := c.Get("X-Trace-ID")
	if traceID != "" {
		ctx = logger.WithTraceID(c.Context(), traceID)
	} else {
		ctx = logger.StartSpan(c.Context())
	}

	// 解析请求体
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error(ctx, "Failed to parse request body: %v", err)
		return c.Status(400).JSON(fiber.Map{
			"code":    400,
			"message": "Invalid request body",
		})
	}

	// 获取认证客户端
	authClient, err := h.getAuthClient(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get auth client: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Internal server error",
		})
	}

	// 调用 gRPC 服务
	resp, err := authClient.Login(ctx, req.Username, req.Password)
	if err != nil {
		logger.Error(ctx, "Login failed: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Login failed",
		})
	}

	// 返回响应
	return c.Status(int(resp.Code)).JSON(fiber.Map{
		"code":          resp.Code,
		"message":       resp.Message,
		"token":         resp.Token,
		"refresh_token": resp.RefreshToken,
		"expires_in":    resp.ExpiresIn,
		"user_info": fiber.Map{
			"user_id":  resp.UserInfo.UserId,
			"username": resp.UserInfo.Username,
			"email":    resp.UserInfo.Email,
			"nickname": resp.UserInfo.Nickname,
			"avatar":   resp.UserInfo.Avatar,
			"roles":    resp.UserInfo.Roles,
		},
	})
}

// VerifyToken 验证令牌
func (h *AuthHandler) VerifyToken(c *fiber.Ctx) error {
	ctx := context.Background()

	// 获取 trace ID
	traceID := c.Get("X-Trace-ID")
	if traceID != "" {
		ctx = logger.WithTraceID(c.Context(), traceID)
	} else {
		ctx = logger.StartSpan(c.Context())
	}

	// 从请求头获取令牌
	token := c.Get("Authorization")
	if token == "" {
		return c.Status(401).JSON(fiber.Map{
			"code":    401,
			"message": "Authorization header is required",
		})
	}

	// 移除 "Bearer " 前缀
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	// 获取认证客户端
	authClient, err := h.getAuthClient(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get auth client: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Internal server error",
		})
	}

	// 调用 gRPC 服务
	resp, err := authClient.VerifyToken(ctx, token)
	if err != nil {
		logger.Error(ctx, "VerifyToken failed: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Verify token failed",
		})
	}

	// 返回响应
	result := fiber.Map{
		"code":    resp.Code,
		"message": resp.Message,
		"valid":   resp.Valid,
	}

	if resp.Valid && resp.UserInfo != nil {
		result["user_info"] = fiber.Map{
			"user_id":  resp.UserInfo.UserId,
			"username": resp.UserInfo.Username,
			"email":    resp.UserInfo.Email,
			"nickname": resp.UserInfo.Nickname,
			"avatar":   resp.UserInfo.Avatar,
			"roles":    resp.UserInfo.Roles,
		}
	}

	return c.Status(int(resp.Code)).JSON(result)
}

// RefreshToken 刷新令牌
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	ctx := context.Background()

	// 获取 trace ID
	traceID := c.Get("X-Trace-ID")
	if traceID != "" {
		ctx = logger.WithTraceID(c.Context(), traceID)
	} else {
		ctx = logger.StartSpan(c.Context())
	}

	// 解析请求体
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error(ctx, "Failed to parse request body: %v", err)
		return c.Status(400).JSON(fiber.Map{
			"code":    400,
			"message": "Invalid request body",
		})
	}

	// 获取认证客户端
	authClient, err := h.getAuthClient(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get auth client: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Internal server error",
		})
	}

	// 调用 gRPC 服务
	resp, err := authClient.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		logger.Error(ctx, "RefreshToken failed: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Refresh token failed",
		})
	}

	// 返回响应
	return c.Status(int(resp.Code)).JSON(fiber.Map{
		"code":          resp.Code,
		"message":       resp.Message,
		"token":         resp.Token,
		"refresh_token": resp.RefreshToken,
		"expires_in":    resp.ExpiresIn,
	})
}

// GetUserInfo 获取用户信息
func (h *AuthHandler) GetUserInfo(c *fiber.Ctx) error {
	ctx := context.Background()

	// 获取 trace ID
	traceID := c.Get("X-Trace-ID")
	if traceID != "" {
		ctx = logger.WithTraceID(c.Context(), traceID)
	} else {
		ctx = logger.StartSpan(c.Context())
	}

	// 从路径参数获取用户ID
	userID := c.Params("id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{
			"code":    400,
			"message": "user_id is required",
		})
	}

	// 获取认证客户端
	authClient, err := h.getAuthClient(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to get auth client: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Internal server error",
		})
	}

	// 调用 gRPC 服务
	resp, err := authClient.GetUserInfo(ctx, userID)
	if err != nil {
		logger.Error(ctx, "GetUserInfo failed: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"code":    500,
			"message": "Get user info failed",
		})
	}

	// 返回响应
	return c.Status(int(resp.Code)).JSON(fiber.Map{
		"code":    resp.Code,
		"message": resp.Message,
		"user_info": fiber.Map{
			"user_id":  resp.UserInfo.UserId,
			"username": resp.UserInfo.Username,
			"email":    resp.UserInfo.Email,
			"nickname": resp.UserInfo.Nickname,
			"avatar":   resp.UserInfo.Avatar,
			"roles":    resp.UserInfo.Roles,
		},
	})
}
