package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// FiberMiddleware Fiber HTTP 指标中间件
func FiberMiddleware(m *Metrics) fiber.Handler {
	if m == nil {
		m = Global()
	}

	return func(c *fiber.Ctx) error {
		start := time.Now()

		if m.HTTPRequestInFlight != nil {
			m.HTTPRequestInFlight.Inc()
			defer m.HTTPRequestInFlight.Dec()
		}

		err := c.Next()

		duration := time.Since(start)
		statusCode := strconv.Itoa(c.Response().StatusCode())
		path := c.Route().Path
		if path == "" {
			path = c.Path()
		}

		m.RecordHTTPRequest(c.Method(), path, statusCode, duration)

		return err
	}
}

// UnaryServerInterceptor gRPC 服务端指标拦截器
func UnaryServerInterceptor(m *Metrics) grpc.UnaryServerInterceptor {
	if m == nil {
		m = Global()
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := status.Code(err).String()

		m.RecordGRPCRequest(info.FullMethod, code, duration)

		return resp, err
	}
}

// StreamServerInterceptor gRPC 流式服务端指标拦截器
func StreamServerInterceptor(m *Metrics) grpc.StreamServerInterceptor {
	if m == nil {
		m = Global()
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		streamType := "unknown"
		if info.IsClientStream && info.IsServerStream {
			streamType = "bidi"
		} else if info.IsClientStream {
			streamType = "client"
		} else if info.IsServerStream {
			streamType = "server"
		}

		if m.GRPCStreamTotal != nil {
			m.GRPCStreamTotal.WithLabelValues(info.FullMethod, streamType).Inc()
		}

		return handler(srv, ss)
	}
}

// UnaryClientInterceptor gRPC 客户端指标拦截器
func UnaryClientInterceptor(m *Metrics) grpc.UnaryClientInterceptor {
	if m == nil {
		m = Global()
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()

		err := invoker(ctx, method, req, reply, cc, opts...)

		duration := time.Since(start)
		code := status.Code(err).String()

		m.RecordGRPCRequest(method, code, duration)

		return err
	}
}

// StreamClientInterceptor gRPC 流式客户端指标拦截器
func StreamClientInterceptor(m *Metrics) grpc.StreamClientInterceptor {
	if m == nil {
		m = Global()
	}

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		streamType := "unknown"
		if desc.ClientStreams && desc.ServerStreams {
			streamType = "bidi"
		} else if desc.ClientStreams {
			streamType = "client"
		} else if desc.ServerStreams {
			streamType = "server"
		}

		if m.GRPCStreamTotal != nil {
			m.GRPCStreamTotal.WithLabelValues(method, streamType).Inc()
		}

		return streamer(ctx, desc, cc, method, opts...)
	}
}
