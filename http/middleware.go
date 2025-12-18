package http

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"quickgo/logger"
	"quickgo/tracing"
)

const (
	// TraceIDHeader trace ID 请求头名称（统一使用此请求头，request_id 和 trace_id 使用同一个值）
	TraceIDHeader = "X-Trace-ID"
	// RequestIDHeader 请求 ID 请求头名称（已废弃，统一使用 TraceIDHeader）
	// Deprecated: 使用 TraceIDHeader 代替
	RequestIDHeader = TraceIDHeader
)

// TraceMiddleware 链路追踪中间件
// 从请求头中提取 trace ID，如果没有则生成新的
// 同时设置 request_id 和 trace_id 为同一个值（用于日志关联和追踪）
// 统一使用 X-Trace-ID 请求头，避免混淆
func TraceMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 从请求头中获取 trace ID（统一使用 X-Trace-ID）
		traceID := c.Get(TraceIDHeader)
		if traceID == "" {
			// 如果没有，生成新的 trace ID
			traceID = logger.GenerateTraceID()
		}

		// 生成新的 span ID
		spanID := logger.GenerateSpanID()

		// 存储到 Locals 中，供后续中间件和处理器使用
		// trace_id 和 request_id 使用同一个值
		c.Locals("trace_id", traceID)
		c.Locals("request_id", traceID)
		c.Locals("span_id", spanID)

		// 将 trace ID 添加到响应头中，方便客户端追踪
		// 统一使用 X-Trace-ID，避免混淆
		c.Set(TraceIDHeader, traceID)

		return c.Next()
	}
}

// LoggingMiddleware 日志中间件
// 记录请求和响应信息，包含链路追踪信息
func LoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// 从 Locals 中获取 trace ID 和 span ID，创建 context
		traceID := GetTraceID(c)
		spanID := GetSpanID(c)
		ctx := context.Background()
		if traceID != "" {
			ctx = logger.WithTrace(ctx, traceID, spanID)
		} else {
			ctx = logger.StartSpan(ctx)
		}

		// 记录请求信息
		logger.Info(ctx, "HTTP request: method=%s, path=%s, ip=%s, user_agent=%s",
			c.Method(),
			c.Path(),
			c.IP(),
			c.Get("User-Agent"),
		)

		// 处理请求
		err := c.Next()

		// 计算耗时
		duration := time.Since(start)
		statusCode := c.Response().StatusCode()

		// 记录响应信息
		if err != nil {
			logger.Error(ctx, "HTTP request failed: method=%s, path=%s, status=%d, duration=%v",
				c.Method(),
				c.Path(),
				statusCode,
				duration,
				err,
			)
		} else {
			logger.Info(ctx, "HTTP request success: method=%s, path=%s, status=%d, duration=%v",
				c.Method(),
				c.Path(),
				statusCode,
				duration,
			)
		}

		return err
	}
}

// RequestIDMiddleware 请求 ID 中间件（已废弃）
// 注意：request_id 和 trace_id 现在使用同一个值，由 TraceMiddleware 统一处理
// 保留此函数以保持向后兼容，但建议直接使用 TraceMiddleware
// Deprecated: 使用 TraceMiddleware 代替，它会同时设置 trace_id 和 request_id
func RequestIDMiddleware() fiber.Handler {
	// 直接返回 TraceMiddleware，因为功能已经合并
	return TraceMiddleware()
}

// RecoveryMiddleware 恢复中间件（自定义实现，作为 fiber 内置 recover 的补充）
// 注意：fiber 已经内置了 recover 中间件，这个可以作为补充或替代
func RecoveryMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				// 从 Locals 中获取 trace ID，创建 context
				traceID := GetTraceID(c)
				ctx := context.Background()
				if traceID != "" {
					ctx = logger.WithTraceID(ctx, traceID)
				} else {
					ctx = logger.StartSpan(ctx)
				}
				logger.Error(ctx, "HTTP panic recovered: %v", r)

				// 返回 500 错误
				c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Internal Server Error",
					"code":  fiber.StatusInternalServerError,
				})
			}
		}()

		return c.Next()
	}
}

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(timeout time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 使用 channel 来检测超时
		done := make(chan error, 1)
		go func() {
			done <- c.Next()
		}()

		select {
		case <-time.After(timeout):
			c.Status(fiber.StatusRequestTimeout).JSON(fiber.Map{
				"error": "Request Timeout",
				"code":  fiber.StatusRequestTimeout,
			})
			return fiber.NewError(fiber.StatusRequestTimeout, "Request Timeout")
		case err := <-done:
			return err
		}
	}
}

// GetTraceID 从 Fiber context 中获取 trace ID
func GetTraceID(c *fiber.Ctx) string {
	// 1. 优先从 Locals 中获取（由 TraceMiddleware 设置）
	if traceID, ok := c.Locals("trace_id").(string); ok && traceID != "" {
		return traceID
	}

	// 2. 如果使用 OpenTelemetry tracing，从 trace_ctx 中提取 trace ID
	if traceCtx, ok := c.Locals("trace_ctx").(context.Context); ok && traceCtx != nil {
		// 尝试从 OpenTelemetry span context 中提取 trace ID
		if traceID := extractTraceIDFromContext(traceCtx); traceID != "" {
			return traceID
		}
	}

	// 3. 从 UserContext 中提取（Fiber 标准方式）
	if userCtx := c.UserContext(); userCtx != nil {
		if traceID := extractTraceIDFromContext(userCtx); traceID != "" {
			return traceID
		}
	}

	return ""
}

// extractTraceIDFromContext 从 context 中提取 trace ID（支持 OpenTelemetry）
func extractTraceIDFromContext(ctx context.Context) string {
	// 1. 优先从 logger context 中获取（由 TraceMiddleware 设置）
	if traceID := logger.GetTraceID(ctx); traceID != "" {
		return traceID
	}

	// 2. 如果启用了 OpenTelemetry tracing，从 span context 中提取
	if tracing.IsEnabled() {
		if traceID := tracing.GetTraceIDFromContext(ctx); traceID != "" {
			return traceID
		}
	}

	return ""
}

// GetSpanID 从 Fiber context 中获取 span ID
func GetSpanID(c *fiber.Ctx) string {
	if spanID, ok := c.Locals("span_id").(string); ok {
		return spanID
	}
	return ""
}

// GetRequestID 从 Fiber context 中获取请求 ID
// 注意：request_id 和 trace_id 使用同一个值
func GetRequestID(c *fiber.Ctx) string {
	// 优先从 request_id 获取
	if requestID, ok := c.Locals("request_id").(string); ok && requestID != "" {
		return requestID
	}
	// 如果没有 request_id，返回 trace_id（它们应该是同一个值）
	return GetTraceID(c)
}
