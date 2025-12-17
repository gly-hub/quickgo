# 快速开始指南

## 前置准备

### 1. 安装依赖工具

```bash
# 安装 Protocol Buffers 编译器
brew install protobuf  # macOS

# 安装 Go 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 确保 GOPATH/bin 在 PATH 中
export PATH=$PATH:$(go env GOPATH)/bin
```

### 2. 启动 etcd

```bash
docker run -d --name etcd \
  -p 2379:2379 \
  -p 2380:2380 \
  quay.io/coreos/etcd:v3.5.13 \
  etcd \
  --advertise-client-urls=http://127.0.0.1:2379 \
  --listen-client-urls=http://0.0.0.0:2379
```

## 启动服务

### 步骤 1: 启动 Auth Server

```bash
cd auth-server

# 1. 生成 proto 代码
make proto

# 2. 初始化 Go 模块依赖
go mod tidy

# 3. 构建服务
make build

# 4. 运行服务
make run
```

服务将在 `0.0.0.0:50051` 启动，并自动注册到 etcd。

### 步骤 2: 启动 Gateway（新终端）

```bash
cd gateway

# 1. 初始化 Go 模块依赖（需要先复制 auth-server 的 proto 生成代码，或使用共享的 proto）
# 注意：gateway 需要访问 auth-server 的 proto 定义
# 可以创建一个共享的 proto 目录，或者复制生成的代码

# 2. 构建服务
make build

# 3. 运行服务
make run
```

服务将在 `0.0.0.0:8080` 启动。

## 测试 API

### 1. 健康检查

```bash
curl http://localhost:8080/health
```

### 2. 用户登录

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

响应示例：
```json
{
  "code": 200,
  "message": "登录成功",
  "token": "...",
  "refresh_token": "...",
  "expires_in": 7200,
  "user_info": {
    "user_id": "1",
    "username": "admin",
    "email": "admin@example.com",
    "nickname": "管理员",
    "avatar": "",
    "roles": ["admin", "user"]
  }
}
```

### 3. 验证令牌

```bash
# 使用上面获取的 token
curl -X GET http://localhost:8080/api/v1/auth/verify \
  -H "Authorization: Bearer <your-token>"
```

### 4. 获取用户信息

```bash
curl -X GET http://localhost:8080/api/v1/auth/user/1 \
  -H "Authorization: Bearer <your-token>"
```

## 注意事项

1. **Proto 代码生成**: auth-server 需要先运行 `make proto` 生成代码
2. **依赖管理**: 运行 `go mod tidy` 更新依赖
3. **服务顺序**: 先启动 auth-server，再启动 gateway
4. **etcd 服务**: 确保 etcd 正在运行

## 项目结构说明

### Auth Server

```
auth-server/
├── cmd/server/main.go          # 服务入口
├── internal/
│   ├── service/auth.go         # 业务逻辑
│   └── handler/auth_handler.go # gRPC 处理器
├── api/proto/
│   ├── auth.proto             # Proto 定义
│   └── gen/                   # 生成的代码（运行 make proto 后）
└── config/configs_local.yaml  # 配置文件
```

### Gateway

```
gateway/
├── cmd/gateway/main.go         # 服务入口
├── internal/
│   ├── handler/auth_handler.go # HTTP 处理器
│   └── service/auth_client.go  # gRPC 客户端
└── config/configs_local.yaml   # 配置文件
```

## 故障排查

### 问题 1: proto 代码未生成

**解决**: 运行 `make proto` 生成代码

### 问题 2: 找不到模块

**解决**: 运行 `go mod tidy` 更新依赖

### 问题 3: etcd 连接失败

**解决**: 确保 etcd 服务正在运行，检查配置中的 etcd 地址

### 问题 4: 服务发现失败

**解决**: 
1. 检查 etcd 是否运行
2. 检查 auth-server 是否已启动并注册
3. 检查 gateway 配置中的服务名称是否正确

