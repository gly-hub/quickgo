# QuickGo 架构说明

## 1. 设计目标

- **显式装配**：只初始化 Option 声明的组件，避免隐式全局副作用。
- **统一生命周期**：`Init` → `Start` → `Stop`/`Run`，Init 失败自动 rollback。
- **可观测性优先**：tracing 尽量先于业务组件；logger 作为默认底座。
- **分层 API**：根包 `quickgo` 面向配置与应用；子包面向实现复用。

## 2. 包分层

```
┌─────────────────────────────────────────────────────────┐
│  Application (example/*, your service)                  │
├─────────────────────────────────────────────────────────┤
│  quickgo (root)                                         │
│    Framework · ConfigLoader · Grpc/HTTP wrappers        │
├──────────────┬──────────────┬───────────────────────────┤
│  grpc/       │  http/       │  db/{gorm,mongodb,redis}  │
│  grpcep/     │  metrics/    │  resilience/              │
│  gerr/       │  tracing/    │  validation/ · logger/    │
└──────────────┴──────────────┴───────────────────────────┘
```

| 层 | 职责 | 典型类型 |
|----|------|----------|
| 应用层 | 业务 handler / service | `main`、内部 package |
| 根包装配层 | YAML 配置、Option、生命周期 | `Framework`, `HTTPServerConfig` |
| 实现层 | 连接、中间件、协议细节 | `grpc.Client`, `http.Server` |
| 横切能力 | 日志、追踪、指标、弹性 | `logger`, `tracing`, `metrics`, `resilience` |

**推荐依赖方向：** 应用 → `quickgo` 根包 → 子包。  
尽量避免应用直接绕过根包配置两套默认值（如 HTTP 的 Enable/Disable 双开关）。

## 3. Framework 启动顺序

`Framework.Init()` 大致顺序（见 `framework.go`）：

1. **Tracing**（若配置）— 其它组件可打点  
2. **Logger**（默认启用）  
3. **Metrics**（若配置）  
4. **gRPC Server**  
5. **gRPC Client Manager**  
6. **HTTP Server**  
7. **GORM / MongoDB / Redis**  
8. **自定义 Component**（`RegisterComponent`）

`Start()` 启动网络监听类组件；`Stop()` 按与初始化相反顺序释放。  
`Run()` 通常阻塞到信号，再优雅退出。

```
           +--------+
  New ---> | Init   | --fail--> rollback (stop partial)
           +---+----+
               | ok
           +---v----+
           | Start  |
           +---+----+
               |
           +---v----+     signal
           |  Wait  | ----------> Stop
           +--------+
```

## 4. 典型请求链路

### 4.1 simple 示例（静态发现）

```
Client
  │  HTTP
  v
Gateway (quickgo HTTPServer + Fiber routes)
  │  gRPC (GrpcClientManager, discovery=static)
  v
RPC Server (GrpcServer + UserService)
  │
  v
In-memory / business logic
```

### 4.2 framework 示例（etcd）

```
Client → Gateway:808x → etcd resolve → Auth gRPC:50051
                              │
                              ├─ GORM (MySQL)
                              └─ Redis (token cache)
```

链路追踪：HTTP 中间件 / gRPC interceptor 注入 OTel span，日志通过 context 带 `trace_id`。

## 5. 配置模型

- 文件名：`configs_{env}.{yaml|json|toml|ini}`  
- 环境：`local` | `develop` | `release` | `production`  
- 覆盖：`DANDELION_ENV` 优先于代码传入的 env  
- 加载：`InitConfig` / `LoadCustomConfig` 将文件反序列化到结构体，再喂给 `ConfigOptionWith*`

组件是否创建，取决于对应 Option 是否传入（及部分 `Enabled` 字段），而不是「配置文件有 key 就自动启动」——需要业务 `main` 显式接线。

## 6. 可观测性

| 信号 | 包 | 暴露方式 |
|------|-----|----------|
| Logs | `logger` | stdout / 文件 JSON |
| Traces | `tracing` | OTLP → Jaeger/Tempo 等 |
| Metrics | `metrics` | HTTP `/metrics`（可配置） |

`resilience` 指标位在 `metrics` 中已预留（RateLimit / CircuitBreaker），但熔断限流拦截器需业务或后续 PR 挂到 gRPC 链路上。

## 7. 扩展点

1. **`Component` 接口**：`Name/Init/Start/Stop/IsEnabled`，注册到 Framework。  
2. **HTTP `AppRouteHandler`**：在 Fiber app 上挂路由。  
3. **gRPC `RegisterService`**：在 server 启动前注册 protobuf 服务。  
4. **自定义 interceptor / middleware**：底层 `grpc` / `http` 包支持追加。

## 8. 已知架构债（规划中）

详见 [PR_PLAN.md](PR_PLAN.md)：

- resilience 未默认接入 Framework  
- 根模块依赖面过大  
- Enable/Disable 双开关语义复杂  
- `grpcep` 含部分业务校验规则  

## 9. 相关文档

- [config-reference.md](config-reference.md)  
- [../README.md](../README.md)  
- [../example/simple/README.md](../example/simple/README.md)  
