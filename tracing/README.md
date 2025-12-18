# 链路追踪（Tracing）

本包提供了完整的链路追踪功能，支持将追踪数据上传到 Jaeger。

## 功能特性

- ✅ OpenTelemetry 标准实现
- ✅ Jaeger 集成（支持 UDP Agent 和 HTTP Collector）
- ✅ gRPC 自动追踪
- ✅ HTTP 自动追踪
- ✅ 数据库操作追踪（GORM）
- ✅ 采样率配置
- ✅ 跨服务链路传播

## 配置说明

### 基本配置

```yaml
tracing:
  enabled: true
  serviceName: "auth-server"
  serviceVersion: "1.0.0"
  environment: "production"
  samplingRate: 1.0  # 采样率：0.0-1.0，1.0 表示采样所有请求
  jaeger:
    enabled: true
    # 方式1：使用 UDP Agent（推荐，性能更好）
    agentHost: "localhost"
    agentPort: 6831
    # 方式2：使用 HTTP Collector（如果需要认证）
    # collectorEndpoint: "http://localhost:14268/api/traces"
    # username: "jaeger"
    # password: "password"
```

### 配置选项

- `enabled`: 是否启用链路追踪
- `serviceName`: 服务名称（用于标识服务）
- `serviceVersion`: 服务版本
- `environment`: 环境名称（dev、staging、prod）
- `samplingRate`: 采样率（0.0-1.0），默认 1.0（采样所有请求）
- `jaeger.enabled`: 是否启用 Jaeger 上传
- `jaeger.agentHost`: Jaeger Agent 主机地址
- `jaeger.agentPort`: Jaeger Agent 端口（UDP，默认 6831）
- `jaeger.collectorEndpoint`: Jaeger Collector HTTP 端点（可选）
- `jaeger.username`: Collector 用户名（如果 Collector 需要认证）
- `jaeger.password`: Collector 密码（如果 Collector 需要认证）

## 使用方法

### 1. 在框架中启用

```go
import (
    "gly-hub/go-dandelion/quickgo"
    "gly-hub/go-dandelion/quickgo/tracing"
)

func main() {
    // 加载配置
    var tracingConfig tracing.Config
    quickgo.LoadCustomConfigKey("tracing", &tracingConfig)
    
    // 创建框架实例
    app, err := quickgo.NewFramework(
        quickgo.ConfigOptionWithApp(appConfig),
        quickgo.ConfigOptionWithLogger(loggerConfig),
        quickgo.ConfigOptionWithTracing(&tracingConfig),
        // ... 其他配置
    )
    if err != nil {
        panic(err)
    }
    
    // 初始化（会自动初始化 tracing）
    if err := app.Init(); err != nil {
        panic(err)
    }
    
    // 启动服务
    if err := app.Start(); err != nil {
        panic(err)
    }
    
    // 等待中断信号（优雅关闭时会自动关闭 tracing）
    app.Wait()
}
```

### 2. gRPC 服务自动追踪

gRPC 服务会自动追踪，无需额外配置。追踪信息会包含：
- 方法名
- 请求/响应大小
- 错误信息
- 执行时间

### 3. HTTP 服务自动追踪

HTTP 服务会自动追踪，需要在 HTTP Server 配置中启用：

```go
httpServerConfig := &quickgo.HTTPServerConfig{
    Enabled: true,
    Address: "0.0.0.0",
    Port:    8080,
    EnableTrace: true,  // 启用追踪
}
```

### 4. 手动创建 Span

```go
import (
    "gly-hub/go-dandelion/quickgo/tracing"
    "go.opentelemetry.io/otel/attribute"
)

func myFunction(ctx context.Context) {
    // 创建新的 span
    ctx, span := tracing.StartSpan(ctx, "my-function")
    defer span.End()
    
    // 设置属性
    span.SetAttributes(
        attribute.String("key", "value"),
        attribute.Int("count", 10),
    )
    
    // 执行操作
    // ...
    
    // 记录错误（如果有）
    if err != nil {
        tracing.SetSpanError(span, err)
        return
    }
}
```

### 5. 数据库操作追踪

GORM 操作会自动追踪，追踪信息会包含：
- SQL 语句
- 执行时间
- 影响行数
- 数据库类型（master/replica）

## Jaeger 部署

### 使用 Docker 部署

```bash
docker run -d \
  --name jaeger \
  -p 6831:6831/udp \
  -p 16686:16686 \
  -p 14268:14268 \
  jaegertracing/all-in-one:latest
```

### 访问 Jaeger UI

部署后访问：http://localhost:16686

## 注意事项

1. **性能影响**：启用追踪会有一定的性能开销，建议在生产环境使用采样率（如 0.1 表示采样 10% 的请求）
2. **存储空间**：追踪数据会占用存储空间，建议配置合适的采样率和数据保留策略
3. **网络延迟**：如果 Jaeger Agent 不可达，可能会影响服务启动（建议配置超时和重试）

