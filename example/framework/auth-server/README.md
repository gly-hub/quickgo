# Auth Server - 认证服务

这是一个使用 QuickGo 框架实现的 gRPC 认证服务示例。

## 项目结构

```
auth-server/
├── cmd/
│   └── server/
│       └── main.go          # 服务入口
├── internal/
│   ├── service/
│   │   └── auth.go          # 业务逻辑
│   └── handler/
│       └── auth_handler.go  # gRPC 处理器
├── api/
│   └── proto/
│       ├── auth.proto       # Proto 定义
│       └── gen/             # 生成的代码（自动生成）
├── config/
│   └── configs_local.yaml   # 配置文件
├── Makefile                 # 构建脚本
├── go.mod                   # Go 模块定义
└── README.md                # 本文件
```

## 功能特性

- 用户登录
- 令牌验证
- 令牌刷新
- 获取用户信息

## 前置要求

1. **安装 Protocol Buffers 编译器**
   ```bash
   brew install protobuf  # macOS
   ```

2. **安装 Go 插件**
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

3. **启动 etcd 服务**
   ```bash
   docker run -d --name etcd \
     -p 2379:2379 \
     -p 2380:2380 \
     quay.io/coreos/etcd:v3.5.13 \
     etcd \
     --advertise-client-urls=http://127.0.0.1:2379 \
     --listen-client-urls=http://0.0.0.0:2379
   ```

## 使用方法

### 1. 生成 Proto 代码

```bash
make proto
```

### 2. 构建服务

```bash
make build
```

### 3. 运行服务

```bash
make run
```

或者直接运行：

```bash
go run ./cmd/server
```

服务将在 `0.0.0.0:50051` 启动，并自动注册到 etcd。

## 测试用户

- 用户名: `admin`, 密码: `admin123`
- 用户名: `user1`, 密码: `user1`

## API 说明

### Login - 用户登录

```protobuf
rpc Login (LoginRequest) returns (LoginResponse);
```

### VerifyToken - 验证令牌

```protobuf
rpc VerifyToken (VerifyTokenRequest) returns (VerifyTokenResponse);
```

### RefreshToken - 刷新令牌

```protobuf
rpc RefreshToken (RefreshTokenRequest) returns (RefreshTokenResponse);
```

### GetUserInfo - 获取用户信息

```protobuf
rpc GetUserInfo (GetUserInfoRequest) returns (GetUserInfoResponse);
```

## 清理

```bash
make clean
```

