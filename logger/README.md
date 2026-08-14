# Logger 日志库

结构化 JSON 日志，支持链路追踪字段与全局 API。

**Import：** `github.com/gly-hub/quickgo/logger`

## 功能特性

- **标准化 JSON 输出**，便于收集与分析
- **Trace ID / Span ID**，与 context 集成
- **级别**：Debug、Info、Warn、Error、Fatal
- **结构化字段**：`WithFields` / `WithField`
- **全局函数**：`logger.Info` 等，无需传递实例
- **可配置**：级别、服务名、版本、文件输出

## 快速开始

### 基本使用

```go
package main

import (
	"context"

	"github.com/gly-hub/quickgo/logger"
)

func main() {
	log, err := logger.NewLogger(logger.Config{
		Level:   logger.LevelInfo,
		Service: "my-service",
		Version: "1.0.0",
	})
	if err != nil {
		panic(err)
	}
	defer log.Close()

	ctx := logger.StartSpan(context.Background())
	log.Info(ctx, "服务启动成功")
	log.Info(ctx, "user=%s id=%d", "alice", 42)
}
```

### 全局日志记录器

```go
package main

import (
	"context"
	"errors"

	"github.com/gly-hub/quickgo/logger"
)

func main() {
	logger.MustInit(logger.Config{
		Level:   logger.LevelInfo,
		Service: "my-service",
		Version: "1.0.0",
	})
	defer logger.Close()

	ctx := logger.StartSpan(context.Background())
	logger.Info(ctx, "使用全局日志记录器")
	logger.Error(ctx, "发生错误: %v", errors.New("boom"))
}
```

> 日志方法签名为 **`fmt.Sprintf` 风格**：`(ctx, format string, args ...interface{})`。  
> `Error` / `Fatal` 若最后一个参数是 `error`，会写入独立的 `error` 字段。

### 链路追踪字段

```go
ctx := logger.StartSpan(context.Background())
logger.Info(ctx, "收到请求")

// 保持 trace_id，生成新 span_id
ctx = logger.StartSpan(ctx)
logger.Info(ctx, "调用下游")
```

### 添加自定义字段

```go
log := logger.WithFields(map[string]interface{}{
	"module": "user",
	"env":    "production",
})
log.Info(ctx, "用户登录")

log = logger.WithField("request_id", "req-123")
log.Info(ctx, "处理请求")
```

## 配置

```go
type Config struct {
	Level      Level  // LevelDebug / LevelInfo / LevelWarn / LevelError / LevelFatal
	Output     string // 文件路径；空则 stdout
	Service    string
	Version    string
	CallerSkip int    // 0 表示动态检测调用栈
}
```

## 日志 JSON 示例

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
    "user_id": 123
  },
  "error": "错误信息（错误日志可选）"
}
```

## Context API

| 函数 | 说明 |
|------|------|
| `StartSpan(ctx)` | 新 span（无 trace 则新建） |
| `WithTraceID` / `WithSpanID` / `WithTrace` | 写入 context |
| `GetTraceID` / `GetSpanID` | 读取 |
| `GenerateTraceID` / `GenerateSpanID` | 生成 ID |

## 全局函数

`Init` / `MustInit` / `SetDefault` / `GetDefault` / `Debug` / `Info` / `Warn` / `Error` / `Fatal` / `WithFields` / `WithField` / `WithContext` / `SetLevel` / `Close`

## 与 Framework 集成

通过 `quickgo.ConfigOptionWithLogger` 启用后，框架会初始化全局 logger；业务侧直接 `logger.Info(ctx, ...)` 即可。

## 更多

- 代码示例：`logger/example.go`
- 框架配置字段见 [docs/config-reference.md](../docs/config-reference.md)
