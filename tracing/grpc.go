package tracing

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryServerInterceptor 创建 gRPC 服务端一元拦截器
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	otelInterceptor := otelgrpc.UnaryServerInterceptor()
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 先调用 otelgrpc 拦截器（它会创建 span）
		resp, err := otelInterceptor(ctx, req, info, handler)

		// 获取 span 并添加 trace_id
		span := trace.SpanFromContext(ctx)
		if span != nil && span.IsRecording() {
			AddTraceIDToSpan(span, ctx)
		}

		return resp, err
	}
}

// StreamServerInterceptor 创建 gRPC 服务端流式拦截器
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	otelInterceptor := otelgrpc.StreamServerInterceptor()
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		// 先调用 otelgrpc 拦截器（它会创建 span）
		err := otelInterceptor(srv, ss, info, handler)

		// 获取 span 并添加 trace_id
		span := trace.SpanFromContext(ctx)
		if span != nil && span.IsRecording() {
			AddTraceIDToSpan(span, ctx)
		}

		return err
	}
}

// UnaryClientInterceptor 创建 gRPC 客户端一元拦截器
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	otelInterceptor := otelgrpc.UnaryClientInterceptor()
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// 先调用 otelgrpc 拦截器（它会创建 span）
		err := otelInterceptor(ctx, method, req, reply, cc, invoker, opts...)

		// 获取 span 并添加 trace_id
		span := trace.SpanFromContext(ctx)
		if span != nil && span.IsRecording() {
			AddTraceIDToSpan(span, ctx)
		}

		return err
	}
}

// StreamClientInterceptor 创建 gRPC 客户端流式拦截器
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	otelInterceptor := otelgrpc.StreamClientInterceptor()
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		// 先调用 otelgrpc 拦截器（它会创建 span）
		stream, err := otelInterceptor(ctx, desc, cc, method, streamer, opts...)

		// 获取 span 并添加 trace_id
		span := trace.SpanFromContext(ctx)
		if span != nil && span.IsRecording() {
			AddTraceIDToSpan(span, ctx)
		}

		return stream, err
	}
}

// ExtractTraceContext 从 gRPC metadata 中提取 trace context
func ExtractTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	// 使用 OpenTelemetry 的 propagator 提取 trace context
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, &metadataCarrier{md: md})
}

// InjectTraceContext 将 trace context 注入到 gRPC metadata 中
func InjectTraceContext(ctx context.Context) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// 使用 OpenTelemetry 的 propagator 注入 trace context
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, &metadataCarrier{md: md})

	return ctx
}

// metadataCarrier 实现 propagation.TextMapCarrier 接口，用于在 gRPC metadata 中传递 trace context
type metadataCarrier struct {
	md metadata.MD
}

func (m *metadataCarrier) Get(key string) string {
	values := m.md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (m *metadataCarrier) Set(key, value string) {
	m.md.Set(key, value)
}

func (m *metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(m.md))
	for k := range m.md {
		keys = append(keys, k)
	}
	return keys
}
