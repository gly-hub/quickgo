package handler

import (
	"context"
	"time"

	"quickgo/db/redis"
	"quickgo/example/framework/gateway/internal/service"
	"quickgo/grpcep"
	"quickgo/logger"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

// AuthHandler HTTP 认证处理器
type AuthHandler struct {
	baseHandler *grpcep.BaseHandler
	authClient  *service.AuthClient
	clientMgr   ClientManager
	cacheRedis  *redis.Client // Redis 缓存客户端（可选）
}

// ClientManager gRPC 客户端管理器接口
type ClientManager interface {
	GetConn(ctx context.Context, serviceName string) (*grpc.ClientConn, error)
}

// NewAuthHandler 创建认证处理器
// clientMgr: gRPC 客户端管理器
// cacheRedis: Redis 缓存客户端（可选，如果为 nil 则不使用缓存）
func NewAuthHandler(clientMgr ClientManager, cacheRedis *redis.Client) *AuthHandler {
	return &AuthHandler{
		baseHandler: &grpcep.BaseHandler{},
		clientMgr:   clientMgr,
		cacheRedis:  cacheRedis,
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

// getCacheKey 生成缓存键
func (h *AuthHandler) getCacheKey(key string) string {
	return "gateway:auth:" + key
}

// getFromCache 从缓存获取数据
func (h *AuthHandler) getFromCache(ctx context.Context, key string) (string, error) {
	if h.cacheRedis == nil {
		return "", nil // 未配置 Redis，返回空
	}

	cacheKey := h.getCacheKey(key)
	val, err := h.cacheRedis.GetClient().Get(ctx, cacheKey).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

// setCache 设置缓存
func (h *AuthHandler) setCache(ctx context.Context, key string, value string, ttl time.Duration) error {
	if h.cacheRedis == nil {
		return nil // 未配置 Redis，忽略
	}

	cacheKey := h.getCacheKey(key)
	return h.cacheRedis.GetClient().Set(ctx, cacheKey, value, ttl).Err()
}

// LoginRequest 登录请求参数
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// Login 用户登录
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	// 创建请求参数
	param := &LoginRequest{}

	// 创建 gRPC handler 函数
	handler := func(ctx context.Context, req *LoginRequest) (interface{}, error) {
		// 获取认证客户端
		authClient, err := h.getAuthClient(ctx)
		if err != nil {
			logger.Error(ctx, "Failed to get auth client: %v", err)
			return nil, err
		}

		// 调用 gRPC 服务
		return authClient.Login(ctx, req.Username, req.Password)
	}

	// 使用 grpcep 的 GRPCCall 方法
	return h.baseHandler.GRPCCall(c, param, handler)
}

// VerifyTokenRequest 验证令牌请求参数
type VerifyTokenRequest struct {
	Token string `json:"token" validate:"required"`
}

// VerifyToken 验证令牌
func (h *AuthHandler) VerifyToken(c *fiber.Ctx) error {
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

	// 创建请求参数
	param := &VerifyTokenRequest{
		Token: token,
	}

	// 创建 gRPC handler 函数
	handler := func(ctx context.Context, req *VerifyTokenRequest) (interface{}, error) {
		// 获取认证客户端
		authClient, err := h.getAuthClient(ctx)
		if err != nil {
			logger.Error(ctx, "Failed to get auth client: %v", err)
			return nil, err
		}

		// 调用 gRPC 服务
		return authClient.VerifyToken(ctx, req.Token)
	}

	// 使用 grpcep 的 GRPCCall 方法
	return h.baseHandler.GRPCCall(c, param, handler)
}

// RefreshTokenRequest 刷新令牌请求参数
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshToken 刷新令牌
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	// 创建请求参数
	param := &RefreshTokenRequest{}

	// 创建 gRPC handler 函数
	handler := func(ctx context.Context, req *RefreshTokenRequest) (interface{}, error) {
		// 获取认证客户端
		authClient, err := h.getAuthClient(ctx)
		if err != nil {
			logger.Error(ctx, "Failed to get auth client: %v", err)
			return nil, err
		}

		// 调用 gRPC 服务
		return authClient.RefreshToken(ctx, req.RefreshToken)
	}

	// 使用 grpcep 的 GRPCCall 方法
	return h.baseHandler.GRPCCall(c, param, handler)
}

// GetUserInfoRequest 获取用户信息请求参数
type GetUserInfoRequest struct {
	UserId string `json:"user_id" validate:"required"`
}

// GetUserInfo 获取用户信息
func (h *AuthHandler) GetUserInfo(c *fiber.Ctx) error {
	// 从路径参数获取用户ID
	userID := c.Params("id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{
			"code":    400,
			"message": "user_id is required",
		})
	}

	// 创建请求参数
	param := &GetUserInfoRequest{
		UserId: userID,
	}

	// 创建 gRPC handler 函数
	handler := func(ctx context.Context, req *GetUserInfoRequest) (interface{}, error) {
		// 获取认证客户端
		authClient, err := h.getAuthClient(ctx)
		if err != nil {
			logger.Error(ctx, "Failed to get auth client: %v", err)
			return nil, err
		}

		// 调用 gRPC 服务
		return authClient.GetUserInfo(ctx, req.UserId)
	}

	// 使用 grpcep 的 GRPCCall 方法
	return h.baseHandler.GRPCCall(c, param, handler)
}
