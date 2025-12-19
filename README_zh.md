# QuickGo 框架

QuickGo 是一个轻量级、模块化的 Go 微服务框架，集成了可观测性功能。

## 功能特性

- 结构化日志记录，支持链路追踪上下文传播
- 分布式追踪（OpenTelemetry/Jaeger）
- 基于 etcd 的服务发现
- API 网关（HTTP 到 gRPC 代理）
- 优雅关闭

## 组件

- **logger**: 结构化日志库，支持 JSON 输出
- **tracing**: OpenTelemetry 集成，用于分布式追踪
- **example/framework**: 完整的微服务示例，包含认证服务和 API 网关

## 快速开始

1. 启动 etcd:
   ```
   docker run -d --name etcd -p 2379:2379 -p 2380:2380 quay.io/coreos/etcd:v3.5.13 etcd --advertise-client-urls=http://127.0.0.1:2379 --listen-client-urls=http://0.0.0.0:2379
   ```

2. 运行认证服务:
   ```
   cd example/framework/auth-server
   make proto && make build && make run
   ```

3. 运行网关（在新终端中）:
   ```
   cd example/framework/gateway
   make build && make run
   ```

4. 测试 API:
   ```
   curl -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d '{"username":"admin","password":"admin123"}'
   ```

## 文档

- [日志库文档](logger/README.md)
- [追踪功能文档](tracing/README.md)
- [框架示例](example/framework/README.md)