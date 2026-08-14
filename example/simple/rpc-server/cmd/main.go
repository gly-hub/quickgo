package main

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/gly-hub/quickgo"
	gen "github.com/gly-hub/quickgo/example/simple/proto/gen"
	"github.com/gly-hub/quickgo/grpcep"
	"github.com/gly-hub/quickgo/logger"
	"github.com/gly-hub/quickgo/tracing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserServiceServer 用户服务实现
type UserServiceServer struct {
	gen.UnimplementedUserServiceServer
	users   map[int64]*gen.User
	mu      sync.RWMutex
	counter int64 // 用户 ID 计数器
}

// NewUserServiceServer 创建用户服务
func NewUserServiceServer() *UserServiceServer {
	s := &UserServiceServer{
		users:   make(map[int64]*gen.User),
		counter: 0,
	}
	// 初始化一些测试数据
	s.initTestData()
	return s
}

func (s *UserServiceServer) initTestData() {
	testUsers := []*gen.User{
		{Id: 1, Username: "admin", Email: "admin@example.com", Phone: "13800000001", Status: 1, CreatedAt: timestamppb.Now()},
		{Id: 2, Username: "test", Email: "test@example.com", Phone: "13800000002", Status: 1, CreatedAt: timestamppb.Now()},
		{Id: 3, Username: "guest", Email: "guest@example.com", Phone: "13800000003", Status: 0, CreatedAt: timestamppb.Now()},
	}
	for _, u := range testUsers {
		s.users[u.Id] = u
	}
	s.counter = 3
}

// GetUser 获取用户信息
func (s *UserServiceServer) GetUser(ctx context.Context, req *gen.GetUserRequest) (*gen.GetUserResponse, error) {
	logger.Info(ctx, "GetUser called: id=%d", req.Id)

	resp := &gen.GetUserResponse{}
	grpcep.InitResponse(&resp)

	s.mu.RLock()
	user, exists := s.users[req.Id]
	s.mu.RUnlock()

	if !exists {
		resp.CommonResp.Code = 40004
		resp.CommonResp.Msg = "user not found"
		return resp, nil
	}

	resp.User = user
	return resp, nil
}

// CreateUser 创建用户
func (s *UserServiceServer) CreateUser(ctx context.Context, req *gen.CreateUserRequest) (*gen.CreateUserResponse, error) {
	logger.Info(ctx, "CreateUser called: username=%s, email=%s", req.Username, req.Email)

	resp := &gen.CreateUserResponse{}
	grpcep.InitResponse(&resp)

	// 参数校验
	if req.Username == "" {
		resp.CommonResp.Code = grpcep.ParamsErrCode
		resp.CommonResp.Msg = "username is required"
		return resp, nil
	}

	// 检查用户名是否已存在
	s.mu.RLock()
	for _, u := range s.users {
		if u.Username == req.Username {
			s.mu.RUnlock()
			resp.CommonResp.Code = 40002
			resp.CommonResp.Msg = "username already exists"
			return resp, nil
		}
	}
	s.mu.RUnlock()

	// 创建新用户
	newID := atomic.AddInt64(&s.counter, 1)
	user := &gen.User{
		Id:        newID,
		Username:  req.Username,
		Email:     req.Email,
		Phone:     req.Phone,
		Status:    1,
		CreatedAt: timestamppb.Now(),
	}

	s.mu.Lock()
	s.users[newID] = user
	s.mu.Unlock()

	resp.User = user
	logger.Info(ctx, "User created: id=%d, username=%s", newID, req.Username)
	return resp, nil
}

// ListUsers 用户列表
func (s *UserServiceServer) ListUsers(ctx context.Context, req *gen.ListUsersRequest) (*gen.ListUsersResponse, error) {
	logger.Info(ctx, "ListUsers called: page=%d, page_size=%d", req.Page, req.PageSize)

	resp := &gen.ListUsersResponse{}
	grpcep.InitResponse(&resp)

	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 转换为切片并排序
	allUsers := make([]*gen.User, 0, len(s.users))
	for _, u := range s.users {
		allUsers = append(allUsers, u)
	}

	total := int32(len(allUsers))
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= total {
		resp.Users = []*gen.User{}
		resp.Total = total
		return resp, nil
	}

	if end > total {
		end = total
	}

	resp.Users = allUsers[start:end]
	resp.Total = total
	return resp, nil
}

// HealthCheck 健康检查
func (s *UserServiceServer) HealthCheck(ctx context.Context, req *gen.HealthCheckRequest) (*gen.HealthCheckResponse, error) {
	resp := &gen.HealthCheckResponse{}
	grpcep.InitResponse(&resp)

	resp.Status = "healthy"
	resp.Version = "1.0.0"
	return resp, nil
}

func main() {
	// 初始化配置（配置文件在上级目录的 config 文件夹）
	quickgo.InitConfig("local", "../config")

	// 加载配置
	var config struct {
		App        quickgo.AppConfig        `yaml:"app"`
		Logger     quickgo.LoggerConfig     `yaml:"logger"`
		GrpcServer quickgo.GrpcServerConfig `yaml:"grpcServer"`
		Tracing    tracing.Config           `yaml:"tracing"`
	}
	quickgo.LoadCustomConfig(&config)

	// 创建框架实例
	app, err := quickgo.NewFramework(
		quickgo.ConfigOptionWithApp(config.App),
		quickgo.ConfigOptionWithLogger(config.Logger),
		quickgo.ConfigOptionWithGrpcServer(&config.GrpcServer),
		quickgo.ConfigOptionWithTracing(&config.Tracing),
	)
	if err != nil {
		panic(err)
	}

	// 初始化
	if err := app.Init(); err != nil {
		panic(err)
	}

	// 注册 gRPC 服务
	userService := NewUserServiceServer()
	app.GrpcServer().RegisterService(func(s *grpc.Server) {
		gen.RegisterUserServiceServer(s, userService)
	})

	// 启动
	if err := app.Start(); err != nil {
		panic(err)
	}

	logger.Info(context.Background(), "RPC Server started successfully")

	// 等待退出
	app.Wait()
}
