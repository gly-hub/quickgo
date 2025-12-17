package handler

import (
	"context"
	gen "gly-hub/go-dandelion/quickgo/example/framework/auth-server/api/proto/gen/api/proto"
	"gly-hub/go-dandelion/quickgo/example/framework/auth-server/internal/service"
	"gly-hub/go-dandelion/quickgo/logger"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthHandler 认证服务处理器
type AuthHandler struct {
	gen.UnimplementedAuthServiceServer
	authService *service.AuthService
}

func (h *AuthHandler) mustEmbedUnimplementedAuthServiceServer() {
	//TODO implement me
	panic("implement me")
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login 用户登录
func (h *AuthHandler) Login(ctx context.Context, req *gen.LoginRequest) (*gen.LoginResponse, error) {
	if req.Username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}
	if req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "password is required")
	}

	resp, err := h.authService.Login(ctx, req.Username, req.Password)
	if err != nil {
		logger.Error(ctx, "Login failed: %v", err)
		return nil, status.Error(codes.Internal, "login failed")
	}

	if resp.Code != 200 {
		return resp, nil
	}

	return resp, nil
}

// VerifyToken 验证令牌
func (h *AuthHandler) VerifyToken(ctx context.Context, req *gen.VerifyTokenRequest) (*gen.VerifyTokenResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	resp, err := h.authService.VerifyToken(ctx, req.Token)
	if err != nil {
		logger.Error(ctx, "VerifyToken failed: %v", err)
		return nil, status.Error(codes.Internal, "verify token failed")
	}

	return resp, nil
}

// RefreshToken 刷新令牌
func (h *AuthHandler) RefreshToken(ctx context.Context, req *gen.RefreshTokenRequest) (*gen.RefreshTokenResponse, error) {
	if req.RefreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	resp, err := h.authService.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		logger.Error(ctx, "RefreshToken failed: %v", err)
		return nil, status.Error(codes.Internal, "refresh token failed")
	}

	return resp, nil
}

// GetUserInfo 获取用户信息
func (h *AuthHandler) GetUserInfo(ctx context.Context, req *gen.GetUserInfoRequest) (*gen.GetUserInfoResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	resp, err := h.authService.GetUserInfo(ctx, req.UserId)
	if err != nil {
		logger.Error(ctx, "GetUserInfo failed: %v", err)
		return nil, status.Error(codes.Internal, "get user info failed")
	}

	return resp, nil
}
