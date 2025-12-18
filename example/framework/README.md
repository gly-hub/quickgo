# Framework 示例

本目录包含使用 QuickGo 框架的完整示例，展示了如何构建微服务架构。

## 项目结构

```
example/framework/
├── auth-server/          # 认证服务（gRPC 服务）
│   ├── cmd/
│   │   └── server/       # 服务入口
│   ├── internal/
│   │   ├── service/      # 业务逻辑
│   │   └── handler/      # gRPC 处理器
│   ├── api/
│   │   └── proto/        # Proto 定义
│   ├── config/           # 配置文件
│   └── Makefile
└── gateway/              # API 网关（HTTP + gRPC）
    ├── cmd/
    │   └── gateway/      # 服务入口
    ├── internal/
    │   ├── handler/      # HTTP 处理器
    │   └── service/      # gRPC 客户端封装
    ├── config/           # 配置文件
    └── Makefile
```

## 架构说明

### Auth Server（认证服务）

- **类型**: gRPC 服务
- **端口**: 50051
- **功能**: 提供用户认证、令牌验证、用户信息查询等服务
- **服务发现**: 通过 etcd 注册服务

### Gateway（API 网关）

- **类型**: HTTP 服务 + gRPC 客户端
- **端口**: 8080
- **功能**: 
  - 对外提供 HTTP REST API
  - 内部通过 gRPC 调用后端服务
  - 服务发现和负载均衡

## 快速开始

### 1. 启动 etcd

```bash
docker run -d --name etcd \
  -p 2379:2379 \
  -p 2380:2380 \
  quay.io/coreos/etcd:v3.5.13 \
  etcd \
  --advertise-client-urls=http://127.0.0.1:2379 \
  --listen-client-urls=http://0.0.0.0:2379
```

### 1.1 启动 Jaeger（可选，用于链路追踪）

```bash
docker run -d --name jaeger \
  -p 6831:6831/udp \
  -p 16686:16686 \
  -p 14268:14268 \
  -p 4317:4317 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

访问 Jaeger UI: http://localhost:16686

### 2. 启动 Auth Server

```bash
cd auth-server
make proto    # 生成 proto 代码
make build    # 构建服务
make run      # 运行服务
```

服务将在 `0.0.0.0:50051` 启动。

### 3. 启动 Gateway

在另一个终端：

```bash
cd gateway
make build    # 构建服务
make run      # 运行服务
```

服务将在 `0.0.0.0:8080` 启动。

### 4. 测试 API

#### 登录

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

#### 验证令牌

```bash
curl -X GET http://localhost:8080/api/v1/auth/verify \
  -H "Authorization: Bearer <your-token>"
```

#### 获取用户信息

```bash
curl -X GET http://localhost:8080/api/v1/auth/user/1 \
  -H "Authorization: Bearer <your-token>"
```

## 测试用户

- 用户名: `admin`, 密码: `admin123`
- 用户名: `user1`, 密码: `user123`

## 项目特点

1. **符合 Go 项目规范**: 使用标准的 Go 项目结构（cmd、internal、api、config）
2. **配置驱动**: 通过 YAML 配置文件管理所有组件
3. **服务发现**: 使用 etcd 进行服务注册和发现
4. **负载均衡**: 支持多种负载均衡策略
5. **链路追踪**: 完整的链路追踪支持（OpenTelemetry + Jaeger）
6. **优雅关闭**: 支持信号处理和优雅关闭

## 链路追踪（Jaeger）

项目已集成 OpenTelemetry 和 Jaeger，支持分布式链路追踪。

### 配置说明

在配置文件中添加 `tracing` 配置：

```yaml
tracing:
  enabled: true
  serviceName: "auth-server"
  serviceVersion: "1.0.0"
  environment: "local"
  samplingRate: 1.0  # 采样率：1.0 表示采样所有请求
  # 方式1：使用 OTLP（推荐）
  otlp:
    enabled: true
    endpoint: "http://localhost:4318"  # HTTP 端点
    useGRPC: false
    insecure: true
  # 方式2：使用 Jaeger Agent（UDP）
  jaeger:
    enabled: false
    agentHost: "localhost"
    agentPort: 6831
```

### 查看追踪数据

1. 启动 Jaeger：`docker run -d -p 16686:16686 -p 4318:4318 jaegertracing/all-in-one:latest`
2. 访问 Jaeger UI：http://localhost:16686
3. 选择服务名称（如 `auth-server` 或 `gateway`）
4. 点击 "Find Traces" 查看追踪数据

### 追踪范围

- ✅ HTTP 请求（Gateway）
- ✅ gRPC 调用（Server 和 Client）
- ✅ 数据库操作（GORM）
- ✅ Redis 操作（可选）

## 目录说明

### cmd/

可执行文件的入口点，每个服务一个目录。

### internal/

内部代码，不会被外部导入。包含：
- `service/`: 业务逻辑层
- `handler/`: 请求处理层

### api/

API 定义，包括：
- `proto/`: Protocol Buffers 定义文件

### config/

配置文件目录，支持多环境配置。

## 扩展

### 添加新的 gRPC 服务

1. 在 `auth-server` 中添加新的 proto 定义
2. 实现 service 和 handler
3. 在 `main.go` 中注册服务

### 添加新的 HTTP 端点

1. 在 `gateway/internal/handler` 中添加新的处理器
2. 在 `gateway/cmd/gateway/main.go` 中注册路由

### 添加新的后端服务

1. 在 `gateway` 中注册新的 gRPC 服务
2. 创建对应的客户端封装
3. 创建 HTTP 处理器

## 注意事项

1. 确保 etcd 服务正在运行
2. 先启动 auth-server，再启动 gateway
3. 确保配置文件路径正确
4. proto 文件需要先编译生成 Go 代码

