# QuickGo 简单示例

这是一个简单的 QuickGo 框架使用示例，包含一个 RPC 用户服务和一个 HTTP 网关。

## 项目结构

```
simple/
├── proto/                  # Protobuf 定义
│   ├── user.proto          # 用户服务接口定义
│   └── gen/                # 生成的代码
├── rpc-server/             # RPC 服务
│   ├── cmd/main.go         # 服务入口
│   └── config/             # 配置文件
├── gateway/                # HTTP 网关
│   ├── cmd/main.go         # 服务入口
│   └── config/             # 配置文件
├── start.sh                # 启动脚本
├── test_api.sh             # API 测试脚本
└── README.md               # 本文件
```

## 快速开始

本示例使用**静态服务发现**，无需 etcd。可选依赖：

```bash
# 仓库根目录：仅在需要链路追踪时启动 Jaeger
make deps-up-minimal
```

也可从仓库根目录一键：

```bash
make example-simple
make example-simple-test
make example-simple-stop
```

### 方式一：使用启动脚本

```bash
# 启动所有服务
./start.sh all

# 运行 API 测试
./test_api.sh

# 停止所有服务
./start.sh stop
```

### 方式二：手动启动

```bash
# 终端 1: 启动 RPC 服务
cd rpc-server/cmd
go run main.go

# 终端 2: 启动网关服务
cd gateway/cmd
go run main.go

# 终端 3: 测试 API
./test_api.sh
```

## API 接口

### 健康检查

```bash
curl http://127.0.0.1:8080/health
```

### 获取用户列表

```bash
curl http://127.0.0.1:8080/api/v1/users/
curl "http://127.0.0.1:8080/api/v1/users/?page=1&page_size=10"
```

### 获取单个用户

```bash
curl http://127.0.0.1:8080/api/v1/users/1
```

### 创建用户

```bash
curl -X POST http://127.0.0.1:8080/api/v1/users/ \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","email":"new@example.com","phone":"13900000001"}'
```

## 服务端口

| 服务 | 端口 | 协议 |
|------|------|------|
| RPC Server (user-service) | 9001 | gRPC |
| Gateway | 8080 | HTTP |

## 测试数据

RPC 服务启动时会初始化以下测试用户：

| ID | 用户名 | 邮箱 | 状态 |
|----|--------|------|------|
| 1 | admin | admin@example.com | 正常 |
| 2 | test | test@example.com | 正常 |
| 3 | guest | guest@example.com | 禁用 |

## 重新生成 Protobuf

如果修改了 proto 文件，使用以下命令重新生成：

```bash
cd proto
protoc --go_out=./gen --go_opt=paths=source_relative \
       --go-grpc_out=./gen --go-grpc_opt=paths=source_relative \
       user.proto
```

## 配置说明

### RPC 服务配置 (rpc-server/config/configs_local.yaml)

- `grpcServer.port`: gRPC 服务端口
- `grpcServer.serviceName`: 服务名称（用于服务发现）

### 网关配置 (gateway/config/configs_local.yaml)

- `httpServer.port`: HTTP 服务端口
- `grpcClient.staticAddresses`: 上游 RPC 服务地址（静态发现模式）
- `grpcClient.poolSize`: gRPC 连接池大小
