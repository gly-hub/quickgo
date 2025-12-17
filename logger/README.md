# Logger 日志库

一个功能完整的 Go 日志库，支持标准化 JSON 格式输出和链路追踪。

## 功能特性

- ✅ **标准化日志格式**：JSON 格式输出，便于日志收集和分析
- ✅ **链路追踪支持**：支持 Trace ID 和 Span ID，方便追踪请求链路
- ✅ **多日志级别**：Debug、Info、Warn、Error、Fatal
- ✅ **结构化日志**：支持添加自定义字段
- ✅ **Context 集成**：与 Go context 深度集成，自动提取链路信息
- ✅ **调用栈信息**：自动记录日志调用位置
- ✅ **全局日志记录器**：提供全局函数，方便使用
- ✅ **灵活配置**：支持文件输出、日志级别等配置

## 快速开始

### 基本使用

```go
package main

import (
    "context"
    "github.com/your-project/logger"
)

func main() {
    // 初始化日志记录器
    config := logger.Config{
        Level:   logger.LevelInfo,
        Service: "my-service",
        Version: "1.0.0",
    }
    
    log, err := logger.NewLogger(config)
    if err != nil {
        panic(err)
    }
    defer log.Close()
    
    // 创建带链路信息的 context
    ctx := logger.StartSpan(context.Background())
    
    // 记录日志
    log.Info(ctx, "服务启动成功")
}
```

### 使用全局日志记录器

```go
package main

import (
    "context"
    "github.com/your-project/logger"
)

func main() {
    // 初始化全局日志记录器
    logger.MustInit(logger.Config{
        Level:   logger.LevelInfo,
        Service: "my-service",
        Version: "1.0.0",
    })
    defer logger.Close()
    
    ctx := logger.StartSpan(context.Background())
    
    // 使用全局函数
    logger.Info(ctx, "使用全局日志记录器")
    logger.Error(ctx, "发生错误", err)
}
```

### 链路追踪

```go
// 在请求入口创建 trace
ctx := logger.StartSpan(context.Background())
log.Info(ctx, "收到请求")

// 调用其他服务时，创建新的 span（保持相同的 trace ID）
ctx = logger.StartSpan(ctx)
log.Info(ctx, "调用用户服务")

// 继续传递 context，所有日志都会包含相同的 trace ID
ctx = logger.StartSpan(ctx)
log.Info(ctx, "查询数据库")
```

### 添加自定义字段

```go
// 方法1：使用 WithFields
log := logger.WithFields(map[string]interface{}{
    "module": "user",
    "env":    "production",
})
log.Info(ctx, "用户登录")

// 方法2：在日志方法中直接传递字段
log.Info(ctx, "用户登录", map[string]interface{}{
    "user_id": 123,
    "ip":      "192.168.1.1",
})

// 方法3：使用 WithField
log = logger.WithField("request_id", "req-123")
log.Info(ctx, "处理请求")
```

### 错误日志

```go
err := errors.New("数据库连接失败")
log.Error(ctx, "无法连接到数据库", err, map[string]interface{}{
    "host":     "localhost",
    "port":     5432,
    "database": "mydb",
    "retries":  3,
})
```

## API 文档

### Logger 结构体

主要的日志记录器对象。

#### 方法

- `NewLogger(config Config) (*Logger, error)` - 创建新的日志记录器
- `Debug(ctx context.Context, msg string, fields ...map[string]interface{})` - 记录调试日志
- `Info(ctx context.Context, msg string, fields ...map[string]interface{})` - 记录信息日志
- `Warn(ctx context.Context, msg string, fields ...map[string]interface{})` - 记录警告日志
- `Error(ctx context.Context, msg string, err error, fields ...map[string]interface{})` - 记录错误日志
- `Fatal(ctx context.Context, msg string, err error, fields ...map[string]interface{})` - 记录致命错误日志（会退出程序）
- `WithFields(fields map[string]interface{}) *Logger` - 添加字段，返回新的 logger
- `WithField(key string, value interface{}) *Logger` - 添加单个字段
- `WithContext(ctx context.Context) *Logger` - 从 context 提取链路信息
- `SetLevel(level Level)` - 设置日志级别
- `GetLevel() Level` - 获取日志级别
- `Close() error` - 关闭日志记录器

### Context 链路追踪

- `StartSpan(ctx context.Context) context.Context` - 开始新的 span
- `WithTraceID(ctx context.Context, traceID string) context.Context` - 设置 trace ID
- `WithSpanID(ctx context.Context, spanID string) context.Context` - 设置 span ID
- `WithTrace(ctx context.Context, traceID, spanID string) context.Context` - 设置 trace ID 和 span ID
- `GetTraceID(ctx context.Context) string` - 获取 trace ID
- `GetSpanID(ctx context.Context) string` - 获取 span ID
- `GenerateTraceID() string` - 生成新的 trace ID
- `GenerateSpanID() string` - 生成新的 span ID

### 全局函数

- `Init(config Config) error` - 初始化全局日志记录器
- `MustInit(config Config)` - 初始化全局日志记录器（失败则 panic）
- `SetDefault(logger *Logger)` - 设置默认日志记录器
- `GetDefault() *Logger` - 获取默认日志记录器
- `Debug(ctx, msg, fields...)` - 全局调试日志
- `Info(ctx, msg, fields...)` - 全局信息日志
- `Warn(ctx, msg, fields...)` - 全局警告日志
- `Error(ctx, msg, err, fields...)` - 全局错误日志
- `Fatal(ctx, msg, err, fields...)` - 全局致命错误日志
- `Close()` - 关闭全局日志记录器

## 日志格式

日志以 JSON 格式输出，包含以下字段：

```json
{
  "timestamp": "2024-01-15T10:30:45.123456789Z",
  "level": "INFO",
  "service": "my-service",
  "version": "1.0.0",
  "trace_id": "a1b2c3d4e5f6g7h8",
  "span_id": "i9j0k1l2",
  "caller": "main.go:42:handleRequest",
  "message": "处理请求成功",
  "fields": {
    "user_id": 123,
    "request_id": "req-123"
  },
  "error": "错误信息（仅错误日志包含）"
}
```

## 配置选项

### Config 结构体

```go
type Config struct {
    Level      Level  // 日志级别：LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal
    Output     string // 输出文件路径，空则输出到 stdout
    Service    string // 服务名称
    Version    string // 服务版本
    CallerSkip int    // 调用栈跳过层数，默认 2
}
```

## 日志级别

- `LevelDebug` - 调试信息，最详细
- `LevelInfo` - 一般信息，默认级别
- `LevelWarn` - 警告信息
- `LevelError` - 错误信息
- `LevelFatal` - 致命错误，会退出程序

## 最佳实践

### 1. 在请求入口创建 Trace

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := logger.StartSpan(r.Context())
    // ... 处理请求
}
```

### 2. 在服务调用时创建新的 Span

```go
func callService(ctx context.Context) {
    ctx = logger.StartSpan(ctx) // 保持相同的 trace ID，生成新的 span ID
    logger.Info(ctx, "调用服务")
}
```

### 3. 使用结构化字段

```go
logger.Info(ctx, "订单创建", map[string]interface{}{
    "order_id": orderID,
    "user_id":  userID,
    "amount":   amount,
})
```

### 4. 错误日志包含上下文

```go
if err != nil {
    logger.Error(ctx, "处理失败", err, map[string]interface{}{
        "operation": "create_order",
        "user_id":   userID,
    })
    return err
}
```

## 与 gRPC 集成

```go
// 在 gRPC 拦截器中使用
func LoggingInterceptor(logger *logger.Logger) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        // 从 metadata 中提取或创建 trace ID
        ctx = logger.StartSpan(ctx)
        
        start := time.Now()
        resp, err := handler(ctx, req)
        duration := time.Since(start)
        
        if err != nil {
            logger.Error(ctx, "gRPC调用失败", err, map[string]interface{}{
                "method":   info.FullMethod,
                "duration": duration.String(),
            })
        } else {
            logger.Info(ctx, "gRPC调用成功", map[string]interface{}{
                "method":   info.FullMethod,
                "duration": duration.String(),
            })
        }
        
        return resp, err
    }
}
```

## 示例

更多示例请参考 `example.go` 文件。

