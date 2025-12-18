package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/team-dandelion/quickgo/db/redis"
	gen "github.com/team-dandelion/quickgo/example/framework/auth-server/api/proto/gen"
	"github.com/team-dandelion/quickgo/example/framework/auth-server/internal/model"
	"github.com/team-dandelion/quickgo/grpcep"
	"github.com/team-dandelion/quickgo/logger"

	gormDB "gorm.io/gorm"
)

// AuthService 认证服务实现
type AuthService struct {
	// GORM 数据库连接（可选）
	db *gormDB.DB
	// Redis 客户端（可选，用于 token 缓存）
	redis *redis.Client
	// 模拟用户数据库（如果未配置数据库，使用内存存储）
	users map[string]*User
	// 模拟令牌存储（如果未配置 Redis，使用内存存储）
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
// db: GORM 数据库连接（可选，如果为 nil 则使用内存存储）
// redisClient: Redis 客户端（可选，如果为 nil 则使用内存存储）
func NewAuthService(db *gormDB.DB, redisClient *redis.Client) *AuthService {
	// 初始化模拟数据（如果未配置数据库，使用内存存储）
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

	service := &AuthService{
		db:     db,
		redis:  redisClient,
		users:  users,
		tokens: make(map[string]*TokenInfo),
	}

	// 如果配置了数据库，初始化表结构并插入初始数据
	if db != nil {
		// 自动迁移表结构
		if err := db.AutoMigrate(&model.UserModel{}); err != nil {
			logger.Error(context.Background(), "Failed to migrate user table: %v", err)
		} else {
			logger.Info(context.Background(), "User table migrated successfully")
		}

		// 插入初始用户数据（如果不存在）
		service.initDefaultUsers(context.Background(), db)
		logger.Info(context.Background(), "AuthService initialized with database")
	} else {
		logger.Info(context.Background(), "AuthService initialized with in-memory storage")
	}

	// 如果配置了 Redis，使用 Redis 存储 token
	if redisClient != nil {
		logger.Info(context.Background(), "AuthService initialized with Redis cache")
	} else {
		logger.Info(context.Background(), "AuthService initialized with in-memory token storage")
	}

	return service
}

// initDefaultUsers 初始化默认用户数据
func (s *AuthService) initDefaultUsers(ctx context.Context, db *gormDB.DB) {
	// 检查是否已有用户
	var count int64
	db.Model(&model.UserModel{}).Count(&count)
	if count > 0 {
		return // 已有用户，不插入
	}

	// 插入默认用户
	defaultUsers := []*model.UserModel{
		{
			UserID:   "1",
			Username: "admin",
			Password: "admin123", // 实际应该使用 bcrypt 等哈希
			Email:    "admin@example.com",
			Nickname: "管理员",
			Avatar:   "",
			Status:   1,
		},
		{
			UserID:   "2",
			Username: "user1",
			Password: "user123",
			Email:    "user1@example.com",
			Nickname: "用户1",
			Avatar:   "",
			Status:   1,
		},
	}

	// 设置角色
	defaultUsers[0].SetRoles([]string{"admin", "user"})
	defaultUsers[1].SetRoles([]string{"user"})

	// 批量插入
	for _, user := range defaultUsers {
		if err := db.Create(user).Error; err != nil {
			logger.Error(ctx, "Failed to create default user %s: %v", user.Username, err)
		} else {
			logger.Info(ctx, "Created default user: %s", user.Username)
		}
	}
}

// Login 用户登录
func (s *AuthService) Login(ctx context.Context, username, password string) (*gen.LoginResponse, error) {
	logger.Info(ctx, "Login attempt: username=%s", username)

	var userModel *model.UserModel
	var err error

	// 从数据库查询用户
	if s.db != nil {
		userModel = &model.UserModel{}
		if err := s.db.WithContext(ctx).Where("username = ? AND status = ?", username, 1).First(userModel).Error; err != nil {
			if err == gormDB.ErrRecordNotFound {
				logger.Warn(ctx, "User not found: username=%s", username)
				resp := newLoginResponse()
				resp.CommonResp.Code = 401
				resp.CommonResp.Msg = "用户名或密码错误"
				return resp, nil
			}
			logger.Error(ctx, "Failed to query user: %v", err)
			resp := newLoginResponse()
			resp.CommonResp.Code = 500
			resp.CommonResp.Msg = "查询用户失败"
			return resp, nil
		}

		// 验证密码（实际应该使用 bcrypt 等哈希比较）
		if userModel.Password != password {
			logger.Warn(ctx, "Invalid password: username=%s", username)
			resp := newLoginResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "用户名或密码错误"
			return resp, nil
		}
	} else {
		// 使用内存存储（向后兼容）
		user, exists := s.users[username]
		if !exists {
			resp := newLoginResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "用户名或密码错误"
			return resp, nil
		}
		if user.Password != password {
			resp := newLoginResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "用户名或密码错误"
			return resp, nil
		}
		// 转换为 UserModel 格式
		userModel = &model.UserModel{
			UserID:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		}
		userModel.SetRoles(user.Roles)
	}

	// 生成令牌
	token, refreshToken, expiresIn, err := s.generateTokens(userModel.UserID)
	if err != nil {
		logger.Error(ctx, "Failed to generate tokens: %v", err)
		resp := newLoginResponse()
		resp.CommonResp.Code = grpcep.InternalErrCode
		resp.CommonResp.Msg = "生成令牌失败"
		return resp, nil
	}

	// 存储令牌信息到 Redis 或内存
	tokenInfo := &TokenInfo{
		UserID:       userModel.UserID,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		RefreshToken: refreshToken,
	}

	if s.redis != nil {
		// 存储到 Redis
		if err := s.saveTokenToRedis(ctx, token, tokenInfo, time.Duration(expiresIn)*time.Second); err != nil {
			logger.Error(ctx, "Failed to save token to Redis: %v", err)
			resp := &gen.LoginResponse{}
			grpcep.InitResponse(&resp)
			resp.CommonResp.Code = grpcep.InternalErrCode
			resp.CommonResp.Msg = "保存令牌失败"
			return resp, nil
		}
		// 同时存储 refresh token 映射
		if err := s.saveRefreshTokenToRedis(ctx, refreshToken, token, time.Duration(expiresIn+3600)*time.Second); err != nil {
			logger.Error(ctx, "Failed to save refresh token to Redis: %v", err)
		}
	} else {
		// 存储到内存（向后兼容）
		s.tokens[token] = tokenInfo
	}

	logger.Info(ctx, "Login success: username=%s, user_id=%s", username, userModel.UserID)

	resp := newLoginResponse()
	resp.CommonResp.Code = grpcep.SuccessCode
	resp.CommonResp.Msg = "登录成功"
	resp.Token = token
	resp.RefreshToken = refreshToken
	resp.ExpiresIn = expiresIn
	resp.UserInfo = &gen.UserInfo{
		UserId:   userModel.UserID,
		Username: userModel.Username,
		Email:    userModel.Email,
		Nickname: userModel.Nickname,
		Avatar:   userModel.Avatar,
		Roles:    userModel.GetRoles(),
	}
	return resp, nil
}

// VerifyToken 验证令牌
func (s *AuthService) VerifyToken(ctx context.Context, token string) (*gen.VerifyTokenResponse, error) {
	logger.Info(ctx, "Verifying token")

	var tokenInfo *TokenInfo
	var err error

	// 从 Redis 或内存获取令牌信息
	if s.redis != nil {
		tokenInfo, err = s.getTokenFromRedis(ctx, token)
		if err != nil {
			logger.Warn(ctx, "Token not found in Redis: %v", err)
			resp := newVerifyTokenResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "令牌无效"
			resp.Valid = false
			return resp, nil
		}
	} else {
		// 从内存获取（向后兼容）
		var exists bool
		tokenInfo, exists = s.tokens[token]
		if !exists {
			resp := newVerifyTokenResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "令牌无效"
			resp.Valid = false
			return resp, nil
		}
	}

	// 检查是否过期
	if time.Now().After(tokenInfo.ExpiresAt) {
		// 删除过期的令牌
		if s.redis != nil {
			s.deleteTokenFromRedis(ctx, token)
		} else {
			delete(s.tokens, token)
		}
		resp := newVerifyTokenResponse()
		resp.CommonResp.Code = 401
		resp.CommonResp.Msg = "令牌已过期"
		resp.Valid = false
		return resp, nil
	}

	// 获取用户信息
	var userModel *model.UserModel
	if s.db != nil {
		// 从数据库查询用户
		userModel = &model.UserModel{}
		if err := s.db.Where("user_id = ? AND status = ?", tokenInfo.UserID, 1).First(userModel).Error; err != nil {
			if err == gormDB.ErrRecordNotFound {
				resp := newVerifyTokenResponse()
				resp.CommonResp.Code = 404
				resp.CommonResp.Msg = "用户不存在"
				resp.Valid = false
				return resp, nil
			}
			logger.Error(ctx, "Failed to query user: %v", err)
			resp := newVerifyTokenResponse()
			resp.CommonResp.Code = 500
			resp.CommonResp.Msg = "查询用户失败"
			resp.Valid = false
			return resp, nil
		}
	} else {
		// 从内存获取（向后兼容）
		user := s.getUserByID(tokenInfo.UserID)
		if user == nil {
			resp := newVerifyTokenResponse()
			resp.CommonResp.Code = 404
			resp.CommonResp.Msg = "用户不存在"
			resp.Valid = false
			return resp, nil
		}
		userModel = &model.UserModel{
			UserID:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		}
		userModel.SetRoles(user.Roles)
	}

	resp := newVerifyTokenResponse()
	resp.CommonResp.Code = 200
	resp.CommonResp.Msg = "令牌有效"
	resp.Valid = true
	resp.UserInfo = &gen.UserInfo{
		UserId:   userModel.UserID,
		Username: userModel.Username,
		Email:    userModel.Email,
		Nickname: userModel.Nickname,
		Avatar:   userModel.Avatar,
		Roles:    userModel.GetRoles(),
	}
	return resp, nil
}

// RefreshToken 刷新令牌
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*gen.RefreshTokenResponse, error) {
	logger.Info(ctx, "Refreshing token")

	var tokenInfo *TokenInfo
	var token string
	var err error

	// 从 Redis 或内存查找对应的令牌
	if s.redis != nil {
		// 从 Redis 获取 refresh token 对应的 access token
		token, err = s.getTokenByRefreshTokenFromRedis(ctx, refreshToken)
		if err != nil {
			logger.Warn(ctx, "Refresh token not found in Redis: %v", err)
			resp := newRefreshTokenResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "刷新令牌无效"
			return resp, nil
		}
		// 获取 token 信息
		tokenInfo, err = s.getTokenFromRedis(ctx, token)
		if err != nil {
			logger.Warn(ctx, "Token not found in Redis: %v", err)
			resp := newRefreshTokenResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "刷新令牌无效"
			return resp, nil
		}
	} else {
		// 从内存查找（向后兼容）
		found := false
		for t, info := range s.tokens {
			if info.RefreshToken == refreshToken {
				tokenInfo = info
				token = t
				found = true
				break
			}
		}
		if !found {
			resp := newRefreshTokenResponse()
			resp.CommonResp.Code = 401
			resp.CommonResp.Msg = "刷新令牌无效"
			return resp, nil
		}
	}

	// 获取用户信息
	var userModel *model.UserModel
	if s.db != nil {
		// 从数据库查询用户
		userModel = &model.UserModel{}
		if err := s.db.Where("user_id = ? AND status = ?", tokenInfo.UserID, 1).First(userModel).Error; err != nil {
			if err == gormDB.ErrRecordNotFound {
				resp := newRefreshTokenResponse()
				resp.CommonResp.Code = 404
				resp.CommonResp.Msg = "用户不存在"
				return resp, nil
			}
			logger.Error(ctx, "Failed to query user: %v", err)
			resp := newRefreshTokenResponse()
			resp.CommonResp.Code = 500
			resp.CommonResp.Msg = "查询用户失败"
			return resp, nil
		}
	} else {
		// 从内存获取（向后兼容）
		user := s.getUserByID(tokenInfo.UserID)
		if user == nil {
			resp := newRefreshTokenResponse()
			resp.CommonResp.Code = 404
			resp.CommonResp.Msg = "用户不存在"
			return resp, nil
		}
		userModel = &model.UserModel{
			UserID:   user.UserID,
			Username: user.Username,
		}
	}

	// 删除旧令牌
	if s.redis != nil {
		s.deleteTokenFromRedis(ctx, token)
		s.deleteRefreshTokenFromRedis(ctx, refreshToken)
	} else {
		delete(s.tokens, token)
	}

	// 生成新令牌
	newToken, newRefreshToken, expiresIn, err := s.generateTokens(userModel.UserID)
	if err != nil {
		logger.Error(ctx, "Failed to generate tokens: %v", err)
		resp := newRefreshTokenResponse()
		resp.CommonResp.Code = 500
		resp.CommonResp.Msg = "生成令牌失败"
		return resp, nil
	}

	// 存储新令牌
	newTokenInfo := &TokenInfo{
		UserID:       userModel.UserID,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		RefreshToken: newRefreshToken,
	}

	if s.redis != nil {
		// 存储到 Redis
		if err := s.saveTokenToRedis(ctx, newToken, newTokenInfo, time.Duration(expiresIn)*time.Second); err != nil {
			logger.Error(ctx, "Failed to save token to Redis: %v", err)
			resp := newRefreshTokenResponse()
			resp.CommonResp.Code = 500
			resp.CommonResp.Msg = "保存令牌失败"
			return resp, nil
		}
		// 存储 refresh token 映射
		if err := s.saveRefreshTokenToRedis(ctx, newRefreshToken, newToken, time.Duration(expiresIn+3600)*time.Second); err != nil {
			logger.Error(ctx, "Failed to save refresh token to Redis: %v", err)
		}
	} else {
		// 存储到内存（向后兼容）
		s.tokens[newToken] = newTokenInfo
	}

	resp := newRefreshTokenResponse()
	resp.CommonResp.Code = 200
	resp.CommonResp.Msg = "刷新成功"
	resp.Token = newToken
	resp.RefreshToken = newRefreshToken
	resp.ExpiresIn = expiresIn
	return resp, nil
}

// GetUserInfo 获取用户信息
func (s *AuthService) GetUserInfo(ctx context.Context, userID string) (*gen.GetUserInfoResponse, error) {
	logger.Info(ctx, "Getting user info: user_id=%s", userID)

	var userModel *model.UserModel

	if s.db != nil {
		// 从数据库查询用户
		userModel = &model.UserModel{}
		if err := s.db.Where("user_id = ? AND status = ?", userID, 1).First(userModel).Error; err != nil {
			if err == gormDB.ErrRecordNotFound {
				resp := newGetUserInfoResponse()
				resp.CommonResp.Code = 404
				resp.CommonResp.Msg = "用户不存在"
				return resp, nil
			}
			logger.Error(ctx, "Failed to query user: %v", err)
			resp := newGetUserInfoResponse()
			resp.CommonResp.Code = 500
			resp.CommonResp.Msg = "查询用户失败"
			return resp, nil
		}
	} else {
		// 从内存获取（向后兼容）
		user := s.getUserByID(userID)
		if user == nil {
			resp := newGetUserInfoResponse()
			resp.CommonResp.Code = 404
			resp.CommonResp.Msg = "用户不存在"
			return resp, nil
		}
		userModel = &model.UserModel{
			UserID:   user.UserID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		}
		userModel.SetRoles(user.Roles)
	}

	resp := newGetUserInfoResponse()
	resp.CommonResp.Code = 200
	resp.CommonResp.Msg = "获取成功"
	resp.UserInfo = &gen.UserInfo{
		UserId:   userModel.UserID,
		Username: userModel.Username,
		Email:    userModel.Email,
		Nickname: userModel.Nickname,
		Avatar:   userModel.Avatar,
		Roles:    userModel.GetRoles(),
	}
	return resp, nil
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

// getUserByID 根据用户ID获取用户（仅用于内存存储的向后兼容）
func (s *AuthService) getUserByID(userID string) *User {
	for _, user := range s.users {
		if user.UserID == userID {
			return user
		}
	}
	return nil
}

// ==================== Redis Token 操作方法 ====================

// getTokenKey 获取 token 的 Redis key
func (s *AuthService) getTokenKey(token string) string {
	return fmt.Sprintf("auth:token:%s", token)
}

// getRefreshTokenKey 获取 refresh token 的 Redis key
func (s *AuthService) getRefreshTokenKey(refreshToken string) string {
	return fmt.Sprintf("auth:refresh:%s", refreshToken)
}

// saveTokenToRedis 保存 token 到 Redis
func (s *AuthService) saveTokenToRedis(ctx context.Context, token string, tokenInfo *TokenInfo, ttl time.Duration) error {
	if s.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	key := s.getTokenKey(token)
	data, err := json.Marshal(tokenInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal token info: %w", err)
	}

	return s.redis.GetClient().Set(ctx, key, data, ttl).Err()
}

// getTokenFromRedis 从 Redis 获取 token
func (s *AuthService) getTokenFromRedis(ctx context.Context, token string) (*TokenInfo, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	key := s.getTokenKey(token)
	data, err := s.redis.GetClient().Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get token from redis: %w", err)
	}

	var tokenInfo TokenInfo
	if err := json.Unmarshal([]byte(data), &tokenInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token info: %w", err)
	}

	return &tokenInfo, nil
}

// deleteTokenFromRedis 从 Redis 删除 token
func (s *AuthService) deleteTokenFromRedis(ctx context.Context, token string) error {
	if s.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	key := s.getTokenKey(token)
	return s.redis.GetClient().Del(ctx, key).Err()
}

// saveRefreshTokenToRedis 保存 refresh token 到 Redis（映射到 access token）
func (s *AuthService) saveRefreshTokenToRedis(ctx context.Context, refreshToken, accessToken string, ttl time.Duration) error {
	if s.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	key := s.getRefreshTokenKey(refreshToken)
	return s.redis.GetClient().Set(ctx, key, accessToken, ttl).Err()
}

// getTokenByRefreshTokenFromRedis 从 Redis 通过 refresh token 获取 access token
func (s *AuthService) getTokenByRefreshTokenFromRedis(ctx context.Context, refreshToken string) (string, error) {
	if s.redis == nil {
		return "", fmt.Errorf("redis client is nil")
	}

	key := s.getRefreshTokenKey(refreshToken)
	token, err := s.redis.GetClient().Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("failed to get token by refresh token: %w", err)
	}

	return token, nil
}

// deleteRefreshTokenFromRedis 从 Redis 删除 refresh token
func (s *AuthService) deleteRefreshTokenFromRedis(ctx context.Context, refreshToken string) error {
	if s.redis == nil {
		return fmt.Errorf("redis client is nil")
	}

	key := s.getRefreshTokenKey(refreshToken)
	return s.redis.GetClient().Del(ctx, key).Err()
}
