# 链路追踪调试指南

## 问题排查步骤

### 1. 检查 Tracing 是否初始化成功

启动服务后，查看日志中是否有以下信息：

```
Tracing initialized: service=auth-server, version=1.0.0, environment=local, otlp_enabled=true, otlp_endpoint=localhost:4318
```

如果没有看到这条日志，说明 tracing 没有初始化。

### 2. 检查配置是否正确加载

确保配置文件中 `tracing.enabled` 为 `true`，并且 `otlp.enabled` 也为 `true`。

### 3. 检查 Jaeger 是否运行

```bash
# 检查 Jaeger 容器是否运行
docker ps | grep jaeger

# 检查端口是否监听
netstat -an | grep 4318
# 或
lsof -i :4318
```

### 4. 检查 OTLP Endpoint 格式

配置文件中的 endpoint 格式：
- ✅ 正确：`localhost:4318` 或 `http://localhost:4318`
- ❌ 错误：`http://localhost:4318/v1/traces`（不要包含路径）

代码会自动解析 URL 格式，提取 `host:port`。

### 5. 检查网络连接

```bash
# 测试是否能连接到 Jaeger OTLP 端点
curl -v http://localhost:4318/v1/traces
# 应该返回 405 Method Not Allowed（这是正常的，说明端点存在）
```

### 6. 检查采样率

确保 `samplingRate` 设置为 `1.0`（采样所有请求），而不是 `0.0`。

### 7. 检查服务是否真的发送了追踪数据

在代码中添加测试 span：

```go
import "gly-hub/go-dandelion/quickgo/tracing"

// 在某个 handler 中
ctx, span := tracing.StartSpan(ctx, "test-operation")
defer span.End()

// 做一些操作
time.Sleep(100 * time.Millisecond)
```

### 8. 查看 Jaeger UI

访问 http://localhost:16686，检查：
- 服务列表中是否有你的服务名称（`auth-server` 或 `gateway`）
- 是否有 traces 数据
- 如果没有，检查 Filters 中的时间范围

## 常见问题

### 问题 1: Tracing 初始化失败

**症状**: 启动时没有看到 "Tracing initialized" 日志

**可能原因**:
- 配置文件中 `tracing.enabled` 为 `false`
- 配置加载失败
- `initTracing` 没有被调用

**解决方法**:
1. 检查配置文件中的 `tracing.enabled` 是否为 `true`
2. 检查 `main.go` 中是否添加了 `ConfigOptionWithTracing`
3. 检查 `framework.go` 的 `Init()` 方法中是否调用了 `initTracing`

### 问题 2: OTLP Exporter 创建失败

**症状**: 启动时报错 "failed to create OTLP exporter"

**可能原因**:
- Endpoint 格式错误
- Jaeger 未运行
- 网络连接问题

**解决方法**:
1. 检查 endpoint 格式（应该是 `host:port` 或 `http://host:port`）
2. 确保 Jaeger 正在运行
3. 检查防火墙设置

### 问题 3: Jaeger 中没有数据

**症状**: Jaeger UI 中没有看到 traces

**可能原因**:
- 采样率为 0
- 没有实际请求
- Span 没有被正确创建

**解决方法**:
1. 检查 `samplingRate` 是否为 `1.0`
2. 发送一些测试请求
3. 检查代码中是否正确使用了 tracing

### 问题 4: 只有部分请求被追踪

**症状**: 只有部分请求出现在 Jaeger 中

**可能原因**:
- 采样率设置过低
- 某些请求没有经过追踪中间件

**解决方法**:
1. 将 `samplingRate` 设置为 `1.0`
2. 确保所有 HTTP 请求都经过 `TraceMiddleware`
3. 确保所有 gRPC 调用都使用了 tracing 拦截器

## 调试命令

### 查看服务日志

```bash
# auth-server
cd example/framework/auth-server
make run

# gateway
cd example/framework/gateway
make run
```

### 测试 HTTP 请求

```bash
# 登录（会触发 gRPC 调用）
curl -X POST http://localhost:8086/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 检查响应头中的 trace ID
curl -v -X POST http://localhost:8086/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' 2>&1 | grep -i trace
```

### 检查 Jaeger 接收到的数据

```bash
# 查看 Jaeger 日志
docker logs jaeger

# 检查 OTLP 端点
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{}'
```

## 验证清单

- [ ] 配置文件中的 `tracing.enabled` 为 `true`
- [ ] 配置文件中的 `otlp.enabled` 为 `true`
- [ ] `otlp.endpoint` 格式正确（`host:port` 或 `http://host:port`）
- [ ] `samplingRate` 设置为 `1.0`
- [ ] Jaeger 容器正在运行
- [ ] 端口 4318 可以访问
- [ ] 启动日志中有 "Tracing initialized" 信息
- [ ] 发送了测试请求
- [ ] Jaeger UI 中可以找到服务名称
- [ ] Jaeger UI 中有 traces 数据

