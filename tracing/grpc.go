package tracing

import (
	"context"
	"errors"
	"io"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryServerInterceptor 创建 gRPC 服务端一元拦截器
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = ExtractTraceContext(ctx)
		ctx, span := StartSpan(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		AddTraceIDToSpan(span, ctx)
		resp, err := handler(ctx, req)
		SetSpanError(span, err)
		return resp, err
	}
}

// StreamServerInterceptor 创建 gRPC 服务端流式拦截器
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ExtractTraceContext(ss.Context())
		ctx, span := StartSpan(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		AddTraceIDToSpan(span, ctx)
		err := handler(srv, &serverStreamWithContext{ServerStream: ss, ctx: ctx})
		SetSpanError(span, err)
		return err
	}
}

type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStreamWithContext) Context() context.Context { return s.ctx }

// UnaryClientInterceptor 创建 gRPC 客户端一元拦截器
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, span := StartSpan(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
		defer span.End()
		AddTraceIDToSpan(span, ctx)
		ctx = InjectTraceContext(ctx)
		err := invoker(ctx, method, req, reply, cc, opts...)
		SetSpanError(span, err)
		return err
	}
}

// StreamClientInterceptor 创建 gRPC 客户端流式拦截器
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx, span := StartSpan(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
		AddTraceIDToSpan(span, ctx)
		ctx = InjectTraceContext(ctx)
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			SetSpanError(span, err)
			span.End()
			return nil, err
		}
		return &tracedClientStream{
			ClientStream:  stream,
			span:          span,
			serverStreams: desc != nil && desc.ServerStreams,
		}, nil
	}
}

type tracedClientStream struct {
	grpc.ClientStream
	span          trace.Span
	serverStreams bool
	once          sync.Once
}

func (s *tracedClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.finish(err)
	}
	return err
}

func (s *tracedClientStream) SendMsg(message interface{}) error {
	err := s.ClientStream.SendMsg(message)
	if err != nil {
		s.finish(err)
	}
	return err
}

func (s *tracedClientStream) RecvMsg(message interface{}) error {
	err := s.ClientStream.RecvMsg(message)
	if err != nil {
		s.finish(err)
	} else if !s.serverStreams {
		// Client-streaming RPCs return their terminal response without a
		// follow-up EOF read in generated CloseAndRecv helpers.
		s.finish(nil)
	}
	return err
}

func (s *tracedClientStream) finish(err error) {
	s.once.Do(func() {
		if errors.Is(err, io.EOF) {
			err = nil
		}
		SetSpanError(s.span, err)
		s.span.End()
	})
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
	} else {
		md = md.Copy()
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

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
