package logger

import (
	"context"
	"errors"
)

// ExampleBasicUsage 基本使用示例
func ExampleBasicUsage() {
	// 初始化日志记录器
	config := Config{
		Level:   LevelInfo,
		Service: "my-service",
		Version: "1.0.0",
	}

	logger, err := NewLogger(config)
	if err != nil {
		panic(err)
	}
	defer logger.Close()

	// 创建带链路信息的 context
	ctx := context.Background()
	ctx = WithTraceID(ctx, GenerateTraceID())
	ctx = WithSpanID(ctx, GenerateSpanID())

	// 记录日志
	logger.Info(ctx, "服务启动成功")
	logger.Debug(ctx, "调试信息: user_id=%d, action=%s", 123, "login")
	logger.Warn(ctx, "警告信息: threshold=%d", 80)
	logger.Error(ctx, "处理请求失败: request_id=%s", "req-123", errors.New("connection timeout"))
}

// ExampleGlobalUsage 全局日志记录器使用示例
func ExampleGlobalUsage() {
	// 初始化全局日志记录器
	MustInit(Config{
		Level:   LevelDebug,
		Service: "my-service",
		Version: "1.0.0",
	})
	defer Close()

	// 创建带链路信息的 context
	ctx := StartSpan(context.Background())

	// 使用全局函数记录日志
	Info(ctx, "使用全局日志记录器")
	Debug(ctx, "调试信息: key=%s", "value")
	Error(ctx, "发生错误: %v", "test", errors.New("test error"))
}

// ExampleWithFields 使用字段的示例
func ExampleWithFields() {
	logger, _ := NewLogger(Config{
		Level:   LevelInfo,
		Service: "my-service",
	})

	ctx := StartSpan(context.Background())

	// 创建带字段的 logger
	fieldLogger := logger.WithFields(map[string]interface{}{
		"module": "user",
		"env":    "production",
	})

	fieldLogger.Info(ctx, "用户登录: user_id=%d, ip=%s", 123, "192.168.1.1")
}

// ExampleTraceChain 链路追踪示例
func ExampleTraceChain() {
	logger, _ := NewLogger(Config{
		Level:   LevelInfo,
		Service: "api-service",
	})

	// 在请求入口创建 trace
	ctx := StartSpan(context.Background())
	logger.Info(ctx, "收到请求: method=%s, path=%s", "GET", "/api/users")

	// 调用服务层，创建新的 span
	ctx = StartSpan(ctx)
	logger.Info(ctx, "调用用户服务")

	// 调用数据库层，创建新的 span
	ctx = StartSpan(ctx)
	logger.Info(ctx, "查询数据库: table=%s, query=%s", "users", "SELECT * FROM users WHERE id = ?")
}

// ExampleErrorHandling 错误处理示例
func ExampleErrorHandling() {
	logger, _ := NewLogger(Config{
		Level:   LevelError,
		Service: "error-service",
	})

	ctx := StartSpan(context.Background())

	// 记录错误
	err := errors.New("数据库连接失败")
	logger.Error(ctx, "无法连接到数据库: host=%s, port=%d, database=%s, retries=%d", "localhost", 5432, "mydb", 3, err)
}

// ExampleContextPropagation Context 传播示例
func ExampleContextPropagation() {
	logger, _ := NewLogger(Config{
		Level:   LevelInfo,
		Service: "service-a",
	})

	// 在服务 A 中创建 trace
	ctx := StartSpan(context.Background())
	logger.Info(ctx, "服务 A 开始处理")

	// 调用服务 B，传递 context
	callServiceB(ctx, logger)
}

func callServiceB(ctx context.Context, logger *Logger) {
	// 创建新的 span，但保持相同的 trace ID
	ctx = StartSpan(ctx)
	logger.Info(ctx, "服务 B 开始处理: service=%s", "service-b")
}

// ExampleStructuredLogging 结构化日志示例
func ExampleStructuredLogging() {
	logger, _ := NewLogger(Config{
		Level:   LevelInfo,
		Service: "structured-service",
	})

	ctx := StartSpan(context.Background())

	// 记录结构化的业务日志
	logger.Info(ctx, "订单创建成功: order_id=%s, user_id=%d, amount=%.2f, currency=%s, items_count=%d, payment_method=%s",
		"ORD-12345", 789, 99.99, "USD", 3, "credit_card")
}

// ExampleLevelControl 日志级别控制示例
func ExampleLevelControl() {
	logger, _ := NewLogger(Config{
		Level:   LevelWarn, // 只记录 Warn 及以上级别
		Service: "level-service",
	})

	ctx := context.Background()

	// 这些日志不会被记录（级别太低）
	logger.Debug(ctx, "调试信息")
	logger.Info(ctx, "信息日志")

	// 这些日志会被记录
	logger.Warn(ctx, "警告信息")
	logger.Error(ctx, "错误信息", errors.New("test error"))
}
