package tracing

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// Middleware 创建 HTTP 链路追踪中间件
func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 从 Fiber context 获取 Go context
		ctx := c.UserContext()
		if ctx == nil {
			ctx = context.Background()
		}

		// 提取 trace context
		propagator := otel.GetTextMapPropagator()
		headers := make(map[string]string)
		c.Request().Header.VisitAll(func(key, value []byte) {
			headers[string(key)] = string(value)
		})
		ctx = propagator.Extract(ctx, &headerCarrier{headers: headers})

		// 创建 span
		tracer := GetTracer()
		spanName := c.Method() + " " + c.Path()
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethodKey.String(c.Method()),
				semconv.HTTPRouteKey.String(c.Path()),
				semconv.HTTPURLKey.String(c.OriginalURL()),
				semconv.NetHostNameKey.String(c.Hostname()),
				attribute.String("net.sock.peer.addr", c.IP()),
			),
		)
		defer span.End()

		// 添加 trace_id 到 span attributes（方便在 Jaeger 中查询）
		AddTraceIDToSpan(span, ctx)

		// 将 context 存储到 Locals 中（供 handler 使用）
		c.Locals("trace_ctx", ctx)
		// 同时设置到 UserContext（Fiber 的标准方式）
		c.SetUserContext(ctx)

		// 处理请求
		err := c.Next()

		// 设置响应状态码
		statusCode := c.Response().StatusCode()
		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(statusCode))

		// 设置 span 状态
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if statusCode >= 400 {
			span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// 注入 trace context 到响应头
		propagator.Inject(ctx, &responseHeaderCarrier{c: c})

		return err
	}
}

// responseHeaderCarrier 实现 propagation.TextMapCarrier 接口，用于在响应头中传递 trace context
type responseHeaderCarrier struct {
	c *fiber.Ctx
}

func (h *responseHeaderCarrier) Get(key string) string {
	return h.c.Get(key)
}

func (h *responseHeaderCarrier) Set(key, value string) {
	h.c.Set(key, value)
}

func (h *responseHeaderCarrier) Keys() []string {
	// 返回所有响应头的键（这里简化处理）
	return []string{}
}

// ExtractTraceContext 从 HTTP 请求中提取 trace context
func ExtractTraceContextFromRequest(c *fiber.Ctx) context.Context {
	ctx := c.Context()

	// 使用 OpenTelemetry 的 propagator 提取 trace context
	propagator := otel.GetTextMapPropagator()
	headers := make(map[string]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})

	return propagator.Extract(ctx, &headerCarrier{headers: headers})
}

// SetSpanError 设置 span 错误状态（在 grpc.go 中也有定义，这里保留以保持一致性）
func SetSpanError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

// SetSpanAttributes 设置 span 属性（在 grpc.go 中也有定义，这里保留以保持一致性）
func SetSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// headerCarrier 实现 propagation.TextMapCarrier 接口，用于在 HTTP headers 中传递 trace context
type headerCarrier struct {
	headers map[string]string
}

func (h *headerCarrier) Get(key string) string {
	return h.headers[key]
}

func (h *headerCarrier) Set(key, value string) {
	h.headers[key] = value
}

func (h *headerCarrier) Keys() []string {
	keys := make([]string, 0, len(h.headers))
	for k := range h.headers {
		keys = append(keys, k)
	}
	return keys
}
