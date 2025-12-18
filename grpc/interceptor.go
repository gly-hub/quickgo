package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/team-dandelion/quickgo/logger"
)

// UnaryInterceptor 一元拦截器类型
type UnaryInterceptor func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)

// StreamInterceptor 流拦截器类型
type StreamInterceptor func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error

// TraceIDMetadataKey trace ID 在 metadata 中的 key
const TraceIDMetadataKey = "x-trace-id"

// SpanIDMetadataKey span ID 在 metadata 中的 key
const SpanIDMetadataKey = "x-span-id"

// LoggingInterceptor 日志拦截器
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// 从 metadata 中提取 trace ID 和 span ID
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			// 提取 trace ID
			if traceIDs := md.Get(TraceIDMetadataKey); len(traceIDs) > 0 && traceIDs[0] != "" {
				ctx = logger.WithTraceID(ctx, traceIDs[0])
			}
			// 提取 span ID（如果有）
			if spanIDs := md.Get(SpanIDMetadataKey); len(spanIDs) > 0 && spanIDs[0] != "" {
				ctx = logger.WithSpanID(ctx, spanIDs[0])
			}
		}

		// 从 context 中提取或创建链路信息（如果没有从 metadata 获取到，则创建新的）
		ctx = logger.StartSpan(ctx)

		// 记录请求信息
		logger.Info(ctx, "gRPC call: method=%s", info.FullMethod)

		// 执行处理
		resp, err := handler(ctx, req)

		// 记录响应信息
		duration := time.Since(start)
		if err != nil {
			logger.Error(ctx, "gRPC call failed: method=%s, duration=%v", info.FullMethod, duration, err)
		} else {
			logger.Info(ctx, "gRPC call success: method=%s, duration=%v", info.FullMethod, duration)
		}

		return resp, err
	}
}

// RecoveryInterceptor 恢复拦截器（防止panic）
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				// 从 metadata 中提取 trace ID（如果存在）
				md, ok := metadata.FromIncomingContext(ctx)
				if ok {
					if traceIDs := md.Get(TraceIDMetadataKey); len(traceIDs) > 0 && traceIDs[0] != "" {
						ctx = logger.WithTraceID(ctx, traceIDs[0])
					}
				}
				// 从 context 中提取或创建链路信息
				ctx = logger.StartSpan(ctx)
				logger.Error(ctx, "panic recovered: method=%s, panic=%v", info.FullMethod, r)
				err = status.Error(codes.Internal, fmt.Sprintf("internal server error: %v", r))
			}
		}()
		return handler(ctx, req)
	}
}

// AuthInterceptor 认证拦截器示例
func AuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 跳过健康检查
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		if authHeader[0] != "Bearer "+token {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(ctx, req)
	}
}

// TimeoutInterceptor 超时拦截器
func TimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return handler(ctx, req)
	}
}

// ChainUnaryInterceptors 链式组合多个一元拦截器
func ChainUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(interceptors...)
}

// wrappedServerStream 包装 ServerStream 以传递包含 trace ID 的 context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// StreamLoggingInterceptor 流式日志拦截器
func StreamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		start := time.Now()

		// 从 metadata 中提取 trace ID 和 span ID
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			// 提取 trace ID
			if traceIDs := md.Get(TraceIDMetadataKey); len(traceIDs) > 0 && traceIDs[0] != "" {
				ctx = logger.WithTraceID(ctx, traceIDs[0])
			}
			// 提取 span ID（如果有）
			if spanIDs := md.Get(SpanIDMetadataKey); len(spanIDs) > 0 && spanIDs[0] != "" {
				ctx = logger.WithSpanID(ctx, spanIDs[0])
			}
		}

		// 从 context 中提取或创建链路信息
		ctx = logger.StartSpan(ctx)

		// 记录请求信息
		logger.Info(ctx, "gRPC stream call: method=%s", info.FullMethod)

		// 创建包装的 stream，将包含 trace ID 的 context 传递给 handler
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// 执行处理
		err := handler(srv, wrappedStream)

		// 记录响应信息
		duration := time.Since(start)
		if err != nil {
			logger.Error(ctx, "gRPC stream call failed: method=%s, duration=%v", info.FullMethod, duration, err)
		} else {
			logger.Info(ctx, "gRPC stream call success: method=%s, duration=%v", info.FullMethod, duration)
		}

		return err
	}
}

// ClientStreamLoggingInterceptor 客户端流式日志拦截器
func ClientStreamLoggingInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		start := time.Now()

		// 从 context 中提取或创建链路信息
		ctx = logger.StartSpan(ctx)

		// 获取 trace ID 和 span ID
		traceID := logger.GetTraceID(ctx)
		spanID := logger.GetSpanID(ctx)

		// 将 trace ID 和 span ID 添加到 metadata 中传递给服务端
		if traceID != "" {
			md, ok := metadata.FromOutgoingContext(ctx)
			if !ok {
				md = metadata.New(nil)
			}
			md = md.Copy()
			md.Set(TraceIDMetadataKey, traceID)
			if spanID != "" {
				md.Set(SpanIDMetadataKey, spanID)
			}
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		// 记录请求信息
		logger.Info(ctx, "gRPC client stream call: method=%s", method)

		// 执行调用
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			duration := time.Since(start)
			logger.Error(ctx, "gRPC client stream call failed: method=%s, duration=%v", method, duration, err)
			return nil, err
		}

		// 记录成功信息
		duration := time.Since(start)
		logger.Info(ctx, "gRPC client stream call success: method=%s, duration=%v", method, duration)

		return stream, nil
	}
}

// ChainStreamInterceptors 链式组合多个流拦截器
func ChainStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) grpc.ServerOption {
	return grpc.ChainStreamInterceptor(interceptors...)
}

// ==================== 客户端拦截器 ====================

// ClientLoggingInterceptor 客户端日志拦截器
func ClientLoggingInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()

		// 从 context 中提取或创建链路信息
		ctx = logger.StartSpan(ctx)

		// 获取 trace ID 和 span ID
		traceID := logger.GetTraceID(ctx)
		spanID := logger.GetSpanID(ctx)

		// 将 trace ID 和 span ID 添加到 metadata 中传递给服务端
		if traceID != "" {
			md, ok := metadata.FromOutgoingContext(ctx)
			if !ok {
				md = metadata.New(nil)
			}
			md = md.Copy()
			md.Set(TraceIDMetadataKey, traceID)
			if spanID != "" {
				md.Set(SpanIDMetadataKey, spanID)
			}
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		// 记录请求信息
		logger.Info(ctx, "gRPC client call: method=%s", method)

		// 执行调用
		err := invoker(ctx, method, req, reply, cc, opts...)

		// 记录响应信息
		duration := time.Since(start)
		if err != nil {
			logger.Error(ctx, "gRPC client call failed: method=%s, duration=%v", method, duration, err)
		} else {
			logger.Info(ctx, "gRPC client call success: method=%s, duration=%v", method, duration)
		}

		return err
	}
}

// ClientAuthInterceptor 客户端认证拦截器
func ClientAuthInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// 添加认证头
		md := metadata.New(map[string]string{
			"authorization": "Bearer " + token,
		})
		ctx = metadata.NewOutgoingContext(ctx, md)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientTimeoutInterceptor 客户端超时拦截器
func ClientTimeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientRecoveryInterceptor 客户端恢复拦截器（防止panic）
func ClientRecoveryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		defer func() {
			if r := recover(); r != nil {
				ctx = logger.StartSpan(ctx)
				logger.Error(ctx, "panic recovered in client: method=%s, panic=%v", method, r)
				err = status.Error(codes.Internal, fmt.Sprintf("internal client error: %v", r))
			}
		}()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ChainUnaryClientInterceptors 链式组合多个客户端一元拦截器
func ChainUnaryClientInterceptors(interceptors ...grpc.UnaryClientInterceptor) grpc.DialOption {
	return grpc.WithChainUnaryInterceptor(interceptors...)
}

// ChainStreamClientInterceptors 链式组合多个客户端流拦截器
func ChainStreamClientInterceptors(interceptors ...grpc.StreamClientInterceptor) grpc.DialOption {
	return grpc.WithChainStreamInterceptor(interceptors...)
}
