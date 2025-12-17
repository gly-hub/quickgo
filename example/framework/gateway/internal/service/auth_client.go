package service

import (
	"context"
	"fmt"

	gen "gly-hub/go-dandelion/quickgo/example/framework/auth-server/api/proto/gen/api/proto"
	"gly-hub/go-dandelion/quickgo/logger"

	"google.golang.org/grpc"
)

// AuthClient 认证服务客户端
type AuthClient struct {
	client gen.AuthServiceClient
}

// NewAuthClient 创建认证客户端
func NewAuthClient(conn *grpc.ClientConn) *AuthClient {
	return &AuthClient{
		client: gen.NewAuthServiceClient(conn),
	}
}

// Login 用户登录
func (c *AuthClient) Login(ctx context.Context, username, password string) (*gen.LoginResponse, error) {
	req := &gen.LoginRequest{
		Username: username,
		Password: password,
	}

	resp, err := c.client.Login(ctx, req)
	if err != nil {
		logger.Error(ctx, "Login RPC call failed: %v", err)
		return nil, fmt.Errorf("login failed: %w", err)
	}

	return resp, nil
}

// VerifyToken 验证令牌
func (c *AuthClient) VerifyToken(ctx context.Context, token string) (*gen.VerifyTokenResponse, error) {
	req := &gen.VerifyTokenRequest{
		Token: token,
	}

	resp, err := c.client.VerifyToken(ctx, req)
	if err != nil {
		logger.Error(ctx, "VerifyToken RPC call failed: %v", err)
		return nil, fmt.Errorf("verify token failed: %w", err)
	}

	return resp, nil
}

// RefreshToken 刷新令牌
func (c *AuthClient) RefreshToken(ctx context.Context, refreshToken string) (*gen.RefreshTokenResponse, error) {
	req := &gen.RefreshTokenRequest{
		RefreshToken: refreshToken,
	}

	resp, err := c.client.RefreshToken(ctx, req)
	if err != nil {
		logger.Error(ctx, "RefreshToken RPC call failed: %v", err)
		return nil, fmt.Errorf("refresh token failed: %w", err)
	}

	return resp, nil
}

// GetUserInfo 获取用户信息
func (c *AuthClient) GetUserInfo(ctx context.Context, userID string) (*gen.GetUserInfoResponse, error) {
	req := &gen.GetUserInfoRequest{
		UserId: userID,
	}

	resp, err := c.client.GetUserInfo(ctx, req)
	if err != nil {
		logger.Error(ctx, "GetUserInfo RPC call failed: %v", err)
		return nil, fmt.Errorf("get user info failed: %w", err)
	}

	return resp, nil
}
