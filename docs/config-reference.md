# 配置参考

本文档描述根包 `quickgo` 与常用子包在 YAML 中的典型字段。  
完整示例见：

- `example/simple/*/config/configs_local.yaml`
- `example/framework/*/config/configs_local.yaml`

环境变量 **`DANDELION_ENV`** 覆盖环境名；配置文件名格式：`configs_{env}.yaml`。

---

## app

```yaml
app:
  name: my-service
  version: "1.0.0"
  env: local   # local | develop | release | production
```

对应：`quickgo.AppConfig`

---

## logger

```yaml
logger:
  enabled: true
  level: debug          # debug | info | warn | error
  output: console       # console | file
  file: ""              # output=file 时的路径
  service: my-service
  version: "1.0.0"
  enableCaller: false  # true 时记录调用者文件和行号；默认关闭以降低热路径开销
  synchronous: false   # 默认异步写出；true 时在当前请求 goroutine 写入
  bufferSize: 1024     # 异步写出队列长度；队列满时保留背压，不丢日志
```

对应：`quickgo.LoggerConfig` → `logger.Config`

---

## grpcServer

```yaml
grpcServer:
  serviceName: user-service
  address: "0.0.0.0"
  port: 9001
  keepAliveTime: "30s"
  keepAliveTimeout: "10s"
  # 注册到 etcd 时需要：
  etcd:
    endpoints:
      - "127.0.0.1:2379"
    dialTimeout: "5s"
    prefix: "/grpc/services"
    ttl: 30
```

对应：`quickgo.GrpcServerConfig`

| 字段 | 说明 |
|------|------|
| `serviceName` | 服务发现注册名 |
| `address` / `port` | 监听地址 |
| `keepAliveTime` / `keepAliveTimeout` | 字符串 duration |
| `maxConnectionIdle` / `maxConnectionAge` / `maxConnectionAgeGrace` | 连接空闲、最大年龄和优雅关闭窗口；空值表示不限制 |
| `etcd` | 非空则向 etcd 注册 |

每个同时运行的 gRPC 服务节点都必须创建自己的 `grpc.NewEtcdRegistry` / `ServiceRegistrar` 实例。注册器持有该节点独立的 lease 与 keepalive 生命周期，不能在多个节点之间共享；同一服务的多个节点使用相同的服务名即可。

---

## grpcClient

```yaml
# 静态发现（simple 示例）
grpcClient:
  discovery: static
  staticAddresses:
    user-service: "127.0.0.1:9001"
  timeout: "10s"
  insecure: true
  keepAliveTime: "30s"
  keepAliveTimeout: "10s"
  loadBalancing: round_robin
  poolSize: 2
  healthCheckInterval: "30s"
  reconnectInterval: "5s"

# TLS（生产推荐；insecure=false 时 tls 块必需）
grpcClient:
  discovery: static
  staticAddresses:
    user-service: "user.internal:9001"
  insecure: false
  tls:
    caFile: "/etc/quickgo/ca.pem"       # 可选；为空时使用系统 CA
    certFile: "/etc/quickgo/client.pem" # 双向 TLS 可选
    keyFile: "/etc/quickgo/client-key.pem"
    serverName: "user.internal"

# etcd 发现（framework gateway）
grpcClient:
  timeout: "10s"
  insecure: true
  keepAliveTime: "10s"
  keepAliveTimeout: "3s"
  loadBalancing: round_robin
  etcd:
    endpoints:
      - "127.0.0.1:2379"
    dialTimeout: "5s"
    prefix: "/grpc/services"
```

对应：`quickgo.GrpcClientConfig`

| 字段 | 说明 |
|------|------|
| `discovery` | `static`、`etcd` 或空（空值直连；旧配置含 `etcd` 块时自动推断为 etcd） |
| `staticAddresses` | `服务名 → host:port` |
| `poolSize` | 每服务连接数，建议 2–4 |
| `healthCheckInterval` | 空或 0 可禁用 |
| `reconnectInterval` | gRPC 连接退避的基础延迟 |
| `tls` | `insecure=false` 时必需；支持系统 CA、自定义 CA 和双向 TLS |

---

## httpServer

```yaml
httpServer:
  enabled: true
  address: "0.0.0.0"
  port: 8080
  enableCORS: true
  enableRecovery: true
  enableLogging: true
  enableTrace: true
  # 显式关闭（与 enable* 同时存在时，disable 优先，见实现）
  disableCORS: false
  disableRecovery: false
  disableLogging: false
  disableTrace: false
  cors:
    allowOrigins: "*"
    allowMethods: "GET,POST,PUT,DELETE,OPTIONS"
    allowHeaders: "*"
    allowCredentials: false
  metricsPath: /metrics
  enableMetricsEndpoint: true  # 默认 false，必须显式启用
  metricsBearerToken: "replace-me" # 可选，建议从密钥管理系统注入
  disableMetricsEndpoint: false
```

对应：`quickgo.HTTPServerConfig`

---

## tracing

推荐 **OTLP**（Jaeger all-in-one 默认开 OTLP）：

```yaml
tracing:
  enabled: true
  serviceName: my-service
  serviceVersion: "1.0.0"
  environment: local
  samplingRate: 1.0       # 零值同样使用默认 1.0
  disableSampling: false  # 显式关闭采样
  otlp:
    enabled: true
    endpoint: "localhost:4318"   # 或 http://localhost:4318
    useGRPC: false
    insecure: true
```

旧 Jaeger exporter 已移除。Jaeger 通过 OTLP `4317`（gRPC）或 `4318`（HTTP）接收 trace。

对应：`tracing.Config`

本地 Jaeger UI：`http://localhost:16686`（`make deps-up` / `deps-up-minimal`）。

---

## metrics

```yaml
# 通过 Framework Option 传入 metrics.Config（也可嵌在 http/grpc 配置中）
# 代码侧：
#   quickgo.ConfigOptionWithMetrics(&metrics.Config{ ... })
```

`metrics.Config` 主要字段：

| 字段 | 默认倾向 |
|------|----------|
| `Namespace` | `quickgo` |
| `EnableHTTP` / `EnableGRPC` / `EnablePool` / `EnableResilience` | true |
| `Disable*` | 显式关闭对应 Enable |
| `Buckets` | Prometheus 默认桶 |

---

## gorm

```yaml
gorm:
  allowUnavailable: false   # 默认 false；true 时不可用实例不阻断启动，但健康检查会失败
  databases:
    - name: "go-admin"          # 代码 GetDB("go-admin")
      master:
        type: mysql             # mysql | postgres | sqlite | sqlserver
        host: "127.0.0.1"
        port: 3306
        user: root
        password: quickgo
        database: go-admin
        charset: utf8mb4
        timezone: Local
      slaves: []                # 可选只读副本
      maxIdleConn: 10
      maxOpenConn: 100
      connMaxLifetime: "30m"
      connMaxIdleTime: "10m"
      enableLog: true
      logParameters: false       # 默认脱敏；生产不要开启参数记录
      logLevel: info
      slowThreshold: 200
```

对应：`db/gorm.GormManagerConfig`  

未配置连接池字段时，QuickGo 默认使用 `maxIdleConn=10`、`maxOpenConn=100`、`connMaxLifetime=30m`、`connMaxIdleTime=10m`；这些限制同样应用于读写分离的副本连接池。
Docker Compose MySQL：`root` / `quickgo`，库名 `go-admin`（见根目录 `docker-compose.yml`）。

PostgreSQL 自动构建 DSN 时默认 `sslmode=require`；仅在可信本地环境显式配置 `sslMode: disable`。

> framework 示例 YAML 中密码可能仍是历史值 `starunion`，使用 compose 时请改成 `quickgo`。

---

## redis

```yaml
redis:
  allowUnavailable: false   # 默认 false；true 时不可用实例不阻断启动，但健康检查会失败
  databases:
    - name: token-cache
      host: "127.0.0.1"
      port: 6379
      password: ""
      db: 0
      poolSize: 10
      minIdleConns: 5
      maxConnAge: "1h"
      poolTimeout: "4s"
      idleTimeout: "5m"
      dialTimeout: "5s"
      readTimeout: "3s"
      writeTimeout: "3s"
      tls: true
      tlsCAFile: "/etc/quickgo/redis-ca.pem"
      tlsCertFile: "/etc/quickgo/redis-client.pem" # 双向 TLS 可选
      tlsKeyFile: "/etc/quickgo/redis-client-key.pem"
      tlsServerName: "redis.internal"
```

对应：`db/redis.RedisManagerConfig`

---

## mongodb

```yaml
mongodb:
  allowUnavailable: false   # 默认 false；true 时不可用实例不阻断启动，但健康检查会失败
  databases:
    - name: log-mongo
      host: "127.0.0.1"
      port: 27017
      username: ""
      password: ""
      database: gateway_logs
      maxPoolSize: 50
      minPoolSize: 5
```

对应：`db/mongodb.MongoManagerConfig`

---

## 装配到 Framework

配置加载后必须通过 Option **显式**挂载：

```go
app, err := quickgo.NewFramework(
	quickgo.ConfigOptionWithApp(cfg.App),
	quickgo.ConfigOptionWithLogger(cfg.Logger),
	quickgo.ConfigOptionWithGrpcServer(&cfg.GrpcServer),
	quickgo.ConfigOptionWithGrpcClient(&cfg.GrpcClient),
	quickgo.ConfigOptionWithHTTPServer(&cfg.HTTPServer),
	quickgo.ConfigOptionWithGorm(&cfg.Gorm),
	quickgo.ConfigOptionWithRedis(&cfg.Redis),
	quickgo.ConfigOptionWithMongoDB(&cfg.MongoDB),
	quickgo.ConfigOptionWithTracing(&cfg.Tracing),
	quickgo.ConfigOptionWithMetrics(&cfg.Metrics),
)
```

未传入的组件不会初始化。

---

## 本地依赖端口（docker-compose）

| 服务 | 端口 |
|------|------|
| etcd | 2379 |
| Jaeger UI | 16686 |
| OTLP HTTP / gRPC | 4318 / 4317 |
| Redis | 6379 |
| MySQL | 3306 |

```bash
make deps-up
make deps-up-minimal   # 仅 etcd + jaeger
make deps-down
```
