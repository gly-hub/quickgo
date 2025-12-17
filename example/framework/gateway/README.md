# Gateway - API 网关服务

这是一个使用 QuickGo 框架实现的 API 网关服务示例，对外提供 HTTP API，内部通过 gRPC 与后端服务通信。

## 项目结构

```
gateway/
├── cmd/
│   └── gateway/
│       └── main.go          # 服务入口
├── internal/
│   ├── handler/
│   │   └── auth_handler.go  # HTTP 处理器
│   └── service/
│       └── auth_client.go   # gRPC 客户端封装
├── config/
│   └── configs_local.yaml   # 配置文件
├── Makefile                 # 构建脚本
├── go.mod                   # Go 模块定义
└── README.md                # 本文件
```

## 功能特性

- HTTP API 网关
- gRPC 服务发现（通过 etcd）
- 负载均衡
- 链路追踪
- 请求日志

## 前置要求

1. **启动 etcd 服务**
   ```bash
   docker run -d --name etcd \
     -p 2379:2379 \
     -p 2380:2380 \
     quay.io/coreos/etcd:v3.5.13 \
     etcd \
     --advertise-client-urls=http://127.0.0.1:2379 \
     --listen-client-urls=http://0.0.0.0:2379
   ```

2. **启动 auth-server 服务**
   参考 `../auth-server/README.md`

## 使用方法

### 1. 构建服务

```bash
make build
```

### 2. 运行服务

```bash
make run
```

或者直接运行：

```bash
go run ./cmd/gateway
```

服务将在 `0.0.0.0:8080` 启动。

## API 端点

### 健康检查

```bash
GET /health
```

### 用户登录

```bash
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}
```

响应：
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

### 验证令牌

```bash
GET /api/v1/auth/verify
Authorization: Bearer <token>
```

### 刷新令牌

```bash
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "..."
}
```

### 获取用户信息

```bash
GET /api/v1/auth/user/:id
Authorization: Bearer <token>
```

## 测试示例

### 1. 登录

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

### 2. 验证令牌

```bash
curl -X GET http://localhost:8080/api/v1/auth/verify \
  -H "Authorization: Bearer <your-token>"
```

### 3. 获取用户信息

```bash
curl -X GET http://localhost:8080/api/v1/auth/user/1 \
  -H "Authorization: Bearer <your-token>"
```

## 清理

```bash
make clean
```

