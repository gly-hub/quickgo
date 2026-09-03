# 链路追踪（Tracing）

本包提供基于 OpenTelemetry 的链路追踪功能，通过 OTLP 将数据上传到 Jaeger、Tempo 或其他兼容后端。

## 功能特性

- ✅ OpenTelemetry 标准实现
- ✅ OTLP HTTP / gRPC exporter
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
  otlp:
    enabled: true
    endpoint: "http://localhost:4318/v1/traces"
    useGRPC: false
    insecure: true
```

### 配置选项

- `enabled`: 是否启用链路追踪
- `serviceName`: 服务名称（用于标识服务）
- `serviceVersion`: 服务版本
- `environment`: 环境名称（dev、staging、prod）
- `samplingRate`: 采样率（0.0-1.0），默认 1.0（采样所有请求）
- `otlp.enabled`: 是否启用 OTLP 上传
- `otlp.endpoint`: `host:port` 或完整的 `http(s)` URL；完整 URL 会保留 path
- `otlp.useGRPC`: 使用 gRPC exporter（默认 `false`，使用 HTTP）
- `otlp.insecure`: `host:port` 配置使用明文连接；URL 配置的协议由 URL 本身决定
- `otlp.headers`: 可选认证请求头

## 使用方法

### 1. 在框架中启用

```go
import (
    "quickgo"
    "github.com/gly-hub/quickgo/tracing"
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
    "github.com/gly-hub/quickgo/tracing"
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
- 慢查询标记

## Jaeger OTLP 部署

### 使用 Docker 部署

```bash
docker run -d \
  --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

### 访问 Jaeger UI

部署后访问：http://localhost:16686

## 注意事项

1. **性能影响**：启用追踪会有一定的性能开销，建议在生产环境使用采样率（如 0.1 表示采样 10% 的请求）
2. **存储空间**：追踪数据会占用存储空间，建议配置合适的采样率和数据保留策略
3. **网络延迟**：为 exporter 配置可达的 collector 和适当的批量导出超时
