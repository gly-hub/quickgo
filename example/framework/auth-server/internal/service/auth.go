package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	gen "gly-hub/go-dandelion/quickgo/example/framework/auth-server/api/proto/gen/api/proto"
	"gly-hub/go-dandelion/quickgo/logger"
)

// AuthService 认证服务实现
type AuthService struct {
	// 模拟用户数据库
	users map[string]*User
	// 模拟令牌存储
	tokens map[string]*TokenInfo
}

// User 用户信息
type User struct {
	UserID   string
	Username string
	Password string
	Email    string
	Nickname string
	Avatar   string
	Roles    []string
}

// TokenInfo 令牌信息
type TokenInfo struct {
	UserID       string
	ExpiresAt    time.Time
	RefreshToken string
}

// NewAuthService 创建认证服务
func NewAuthService() *AuthService {
	// 初始化模拟数据
	users := map[string]*User{
		"admin": {
			UserID:   "1",
			Username: "admin",
			Password: "admin123", // 实际应该使用哈希
			Email:    "admin@example.com",
			Nickname: "管理员",
			Avatar:   "",
			Roles:    []string{"admin", "user"},
		},
		"user1": {
			UserID:   "2",
			Username: "user1",
			Password: "user123",
			Email:    "user1@example.com",
			Nickname: "用户1",
			Avatar:   "",
			Roles:    []string{"user"},
		},
	}

	return &AuthService{
		users:  users,
		tokens: make(map[string]*TokenInfo),
	}
}

// Login 用户登录
func (s *AuthService) Login(ctx context.Context, username, password string) (*gen.LoginResponse, error) {
	logger.Info(ctx, "Login attempt: username=%s", username)

	// 查找用户
	user, exists := s.users[username]
	if !exists {
		return &gen.LoginResponse{
			Code:    401,
			Message: "用户名或密码错误",
		}, nil
	}

	// 验证密码（实际应该使用哈希比较）
	if user.Password != password {
		return &gen.LoginResponse{
			Code:    401,
			Message: "用户名或密码错误",
		}, nil
	}

	// 生成令牌
	token, refreshToken, expiresIn, err := s.generateTokens(user.UserID)
	if err != nil {
		logger.Error(ctx, "Failed to generate tokens: %v", err)
		return &gen.LoginResponse{
			Code:    500,
			Message: "生成令牌失败",
		}, nil
	}

	// 存储令牌信息
	s.tokens[token] = &TokenInfo{
		UserID:       user.UserID,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		RefreshToken: refreshToken,
	}

	logger.Info(ctx, "Login success: username=%s, user_id=%s", username, user.UserID)

	return &gen.LoginResponse{
		Code:         200,
		Message:      "登录成功",
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		UserInfo: &gen.UserInfo{
			UserId:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Roles:    user.Roles,
		},
	}, nil
}

// VerifyToken 验证令牌
func (s *AuthService) VerifyToken(ctx context.Context, token string) (*gen.VerifyTokenResponse, error) {
	logger.Info(ctx, "Verifying token")

	tokenInfo, exists := s.tokens[token]
	if !exists {
		return &gen.VerifyTokenResponse{
			Code:    401,
			Message: "令牌无效",
			Valid:   false,
		}, nil
	}

	// 检查是否过期
	if time.Now().After(tokenInfo.ExpiresAt) {
		delete(s.tokens, token)
		return &gen.VerifyTokenResponse{
			Code:    401,
			Message: "令牌已过期",
			Valid:   false,
		}, nil
	}

	// 获取用户信息
	user := s.getUserByID(tokenInfo.UserID)
	if user == nil {
		return &gen.VerifyTokenResponse{
			Code:    404,
			Message: "用户不存在",
			Valid:   false,
		}, nil
	}

	return &gen.VerifyTokenResponse{
		Code:    200,
		Message: "令牌有效",
		Valid:   true,
		UserInfo: &gen.UserInfo{
			UserId:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Roles:    user.Roles,
		},
	}, nil
}

// RefreshToken 刷新令牌
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*gen.RefreshTokenResponse, error) {
	logger.Info(ctx, "Refreshing token")

	// 查找对应的令牌
	var tokenInfo *TokenInfo
	var token string
	for t, info := range s.tokens {
		if info.RefreshToken == refreshToken {
			tokenInfo = info
			token = t
			break
		}
	}

	if tokenInfo == nil {
		return &gen.RefreshTokenResponse{
			Code:    401,
			Message: "刷新令牌无效",
		}, nil
	}

	// 获取用户信息
	user := s.getUserByID(tokenInfo.UserID)
	if user == nil {
		return &gen.RefreshTokenResponse{
			Code:    404,
			Message: "用户不存在",
		}, nil
	}

	// 删除旧令牌
	delete(s.tokens, token)

	// 生成新令牌
	newToken, newRefreshToken, expiresIn, err := s.generateTokens(user.UserID)
	if err != nil {
		logger.Error(ctx, "Failed to generate tokens: %v", err)
		return &gen.RefreshTokenResponse{
			Code:    500,
			Message: "生成令牌失败",
		}, nil
	}

	// 存储新令牌
	s.tokens[newToken] = &TokenInfo{
		UserID:       user.UserID,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		RefreshToken: newRefreshToken,
	}

	return &gen.RefreshTokenResponse{
		Code:         200,
		Message:      "刷新成功",
		Token:        newToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    expiresIn,
	}, nil
}

// GetUserInfo 获取用户信息
func (s *AuthService) GetUserInfo(ctx context.Context, userID string) (*gen.GetUserInfoResponse, error) {
	logger.Info(ctx, "Getting user info: user_id=%s", userID)

	user := s.getUserByID(userID)
	if user == nil {
		return &gen.GetUserInfoResponse{
			Code:    404,
			Message: "用户不存在",
		}, nil
	}

	return &gen.GetUserInfoResponse{
		Code:    200,
		Message: "获取成功",
		UserInfo: &gen.UserInfo{
			UserId:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Roles:    user.Roles,
		},
	}, nil
}

// generateTokens 生成令牌
func (s *AuthService) generateTokens(userID string) (token, refreshToken string, expiresIn int64, err error) {
	// 生成访问令牌
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", 0, err
	}
	token = base64.URLEncoding.EncodeToString(tokenBytes)

	// 生成刷新令牌
	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil {
		return "", "", 0, err
	}
	refreshToken = base64.URLEncoding.EncodeToString(refreshBytes)

	// 设置过期时间（2小时）
	expiresIn = 7200

	return token, refreshToken, expiresIn, nil
}

// getUserByID 根据用户ID获取用户
func (s *AuthService) getUserByID(userID string) *User {
	for _, user := range s.users {
		if user.UserID == userID {
			return user
		}
	}
	return nil
}
