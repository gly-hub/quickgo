package handler

import (
	"context"
	gen "github.com/team-dandelion/quickgo/example/framework/auth-server/api/proto/gen"
	"github.com/team-dandelion/quickgo/example/framework/auth-server/internal/service"
	"github.com/team-dandelion/quickgo/grpcep"
	"github.com/team-dandelion/quickgo/logger"

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

	resp := &gen.LoginResponse{}
	grpcep.InitResponse(&resp)

	respResult, err := h.authService.Login(ctx, req.Username, req.Password)
	if err != nil {
		logger.Error(ctx, "Login failed: %v", err)
		resp.CommonResp.Code = grpcep.InternalErrCode
		resp.CommonResp.Msg = "登录失败"
		return resp, nil
	}

	// 复制响应数据
	resp.CommonResp = respResult.CommonResp
	resp.Token = respResult.Token
	resp.RefreshToken = respResult.RefreshToken
	resp.ExpiresIn = respResult.ExpiresIn
	resp.UserInfo = respResult.UserInfo

	return resp, nil
}

// VerifyToken 验证令牌
func (h *AuthHandler) VerifyToken(ctx context.Context, req *gen.VerifyTokenRequest) (*gen.VerifyTokenResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	resp := &gen.VerifyTokenResponse{}
	grpcep.InitResponse(&resp)

	respResult, err := h.authService.VerifyToken(ctx, req.Token)
	if err != nil {
		logger.Error(ctx, "VerifyToken failed: %v", err)
		resp.CommonResp.Code = grpcep.InternalErrCode
		resp.CommonResp.Msg = "验证令牌失败"
		return resp, nil
	}

	// 复制响应数据
	resp.CommonResp = respResult.CommonResp
	resp.Valid = respResult.Valid
	resp.UserInfo = respResult.UserInfo

	return resp, nil
}

// RefreshToken 刷新令牌
func (h *AuthHandler) RefreshToken(ctx context.Context, req *gen.RefreshTokenRequest) (*gen.RefreshTokenResponse, error) {
	if req.RefreshToken == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	resp := &gen.RefreshTokenResponse{}
	grpcep.InitResponse(&resp)

	respResult, err := h.authService.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		logger.Error(ctx, "RefreshToken failed: %v", err)
		resp.CommonResp.Code = grpcep.InternalErrCode
		resp.CommonResp.Msg = "刷新令牌失败"
		return resp, nil
	}

	// 复制响应数据
	resp.CommonResp = respResult.CommonResp
	resp.Token = respResult.Token
	resp.RefreshToken = respResult.RefreshToken
	resp.ExpiresIn = respResult.ExpiresIn

	return resp, nil
}

// GetUserInfo 获取用户信息
func (h *AuthHandler) GetUserInfo(ctx context.Context, req *gen.GetUserInfoRequest) (*gen.GetUserInfoResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	resp := &gen.GetUserInfoResponse{}
	grpcep.InitResponse(&resp)

	respResult, err := h.authService.GetUserInfo(ctx, req.UserId)
	if err != nil {
		logger.Error(ctx, "GetUserInfo failed: %v", err)
		resp.CommonResp.Code = grpcep.InternalErrCode
		resp.CommonResp.Msg = "获取用户信息失败"
		return resp, nil
	}

	// 复制响应数据
	resp.CommonResp = respResult.CommonResp
	resp.UserInfo = respResult.UserInfo

	return resp, nil
}
