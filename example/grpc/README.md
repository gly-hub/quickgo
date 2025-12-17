# gRPC 示例

这是一个完整的 gRPC 服务端和客户端示例，演示了如何使用 QuickGo 的 gRPC 封装库。

## 目录结构

```
example/
├── proto/           # Proto 定义文件
│   ├── hello.proto  # Hello 服务定义
│   └── gen/        # 生成的 Go 代码（自动生成）
├── server/          # 服务端实现
│   └── main.go
├── client/          # 客户端实现
│   └── main.go
├── Makefile         # 构建脚本
└── README.md        # 本文件
```

## 前置要求

1. **安装 Protocol Buffers 编译器**
   ```bash
   # macOS
   brew install protobuf
   
   # 或使用其他包管理器
   ```

2. **安装 Go 插件**
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

3. **确保 GOPATH/bin 在 PATH 中**
   ```bash
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

4. **启动 etcd 服务**（用于服务发现）
   ```bash
   # 使用 Docker
   docker run -d --name etcd \
     -p 2379:2379 \
     -p 2380:2380 \
     quay.io/coreos/etcd:v3.5.13 \
     etcd \
     --advertise-client-urls=http://127.0.0.1:2379 \
     --listen-client-urls=http://0.0.0.0:2379
   
   # 或使用本地安装的 etcd
   etcd --listen-client-urls=http://127.0.0.1:2379 --advertise-client-urls=http://127.0.0.1:2379
   ```

## 使用方法

### 1. 启动 etcd 服务

确保 etcd 服务正在运行（默认地址：`127.0.0.1:2379`）

```bash
# 检查 etcd 是否运行
etcdctl endpoint health
```

如果 etcd 不在默认地址，可以通过环境变量指定：

```bash
export ETCD_ENDPOINT=your-etcd-address:2379
```

### 2. 生成 Proto 代码

```bash
make proto
```

这会从 `proto/hello.proto` 生成 Go 代码到 `proto/gen/` 目录。

### 3. 构建和运行服务端

```bash
# 构建
make build-server

# 或直接运行
make run-server
```

服务端将：
- 在 `0.0.0.0:50051` 端口启动
- 自动注册到 etcd（服务名：`hello-service`）
- 启动心跳保持服务活跃

**注意**：如果需要指定服务器 IP（用于注册到 etcd），可以设置环境变量：

```bash
export SERVER_IP=192.168.1.100
make run-server
```

### 4. 构建和运行客户端

在另一个终端中：

```bash
# 构建
make build-client

# 或直接运行
make run-client
```

客户端将：
- 从 etcd 自动发现服务地址
- 使用轮询负载均衡策略
- 连接到服务端并执行以下测试：
  - 简单 RPC 调用 (`SayHello`)
  - 服务端流式调用 (`SayHelloStream`)
  - 客户端流式调用 (`SayHelloClientStream`)
  - 双向流式调用 (`SayHelloBidiStream`)

## 手动构建

如果不想使用 Makefile：

```bash
# 1. 生成 proto 代码
mkdir -p proto/gen
protoc --go_out=proto/gen \
  --go_opt=paths=source_relative \
  --go-grpc_out=proto/gen \
  --go-grpc_opt=paths=source_relative \
  proto/hello.proto

# 2. 构建 server
go build -o bin/server ./server

# 3. 构建 client
go build -o bin/client ./client

# 4. 运行
./bin/server    # 在一个终端
./bin/client    # 在另一个终端
```

## 测试流程

1. **启动服务端**
   ```bash
   make run-server
   # 或
   ./bin/server
   ```
   服务端会在 `:50051` 端口监听

2. **运行客户端**（在另一个终端）
   ```bash
   make run-client
   # 或
   ./bin/client
   ```

3. **观察输出**
   - 服务端会显示接收到的请求日志
   - 客户端会显示所有测试的输出结果

## 清理

```bash
make clean
```

这会删除所有生成的文件和构建产物。

## 服务定义

### HelloService

定义了 4 种类型的 RPC 方法：

1. **SayHello** - 简单的一元 RPC
   - 客户端发送一个请求，服务端返回一个响应

2. **SayHelloStream** - 服务端流式 RPC
   - 客户端发送一个请求，服务端返回多个响应

3. **SayHelloClientStream** - 客户端流式 RPC
   - 客户端发送多个请求，服务端返回一个响应

4. **SayHelloBidiStream** - 双向流式 RPC
   - 客户端和服务端都可以发送多个消息

## 功能特性

- ✅ 使用 QuickGo 的 gRPC 封装库
- ✅ 集成 logger 库进行日志输出
- ✅ 支持所有 4 种 gRPC 调用模式
- ✅ 错误处理和日志记录
- ✅ 优雅关闭（服务端）
- ✅ **etcd 服务发现和注册**
- ✅ **自动心跳保持服务活跃**
- ✅ **负载均衡（轮询策略）**

## 配置说明

### 环境变量

- `ETCD_ENDPOINT`: etcd 服务地址（默认：`127.0.0.1:2379`）
- `SERVER_IP`: 服务器 IP 地址（用于注册到 etcd，默认自动检测）

### 服务配置

在代码中定义的常量：

- `serviceName`: 服务名称（`hello-service`）
- `servicePort`: 服务端口（`50051`）
- `etcdPrefix`: etcd 前缀（`/grpc/services`）
- `etcdTTL`: etcd 租约 TTL（`30` 秒）

## 扩展

你可以基于这个示例扩展：

1. **添加更多服务方法**
   - 编辑 `proto/hello.proto`
   - 重新生成代码：`make proto`
   - 在 server 和 client 中实现新方法

2. **修改负载均衡策略**
   - 在 `client/main.go` 中修改 `LoadBalancing` 字段
   - 可选：`PolicyRoundRobin`、`PolicyPickFirst`、`PolicyWeightedRoundRobin`

3. **添加 TLS**
   - 在 `grpc.Config` 和 `grpc.ClientConfig` 中配置 TLS

4. **添加拦截器**
   - 在 server 和 client 配置中添加自定义拦截器

5. **多实例测试**
   - 启动多个服务端实例（使用不同的端口）
   - 客户端会自动从 etcd 发现所有实例
   - 请求会在多个实例间负载均衡

