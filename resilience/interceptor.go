package resilience

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryClientCircuitBreaker gRPC 客户端熔断拦截器
func UnaryClientCircuitBreaker(manager *CircuitBreakerManager) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		cb := manager.Get(method)

		return cb.Execute(ctx, func(ctx context.Context) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		})
	}
}

// UnaryServerRateLimiter gRPC 服务端限流拦截器
func UnaryServerRateLimiter(limiter RateLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !limiter.Allow() {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(ctx, req)
	}
}

// UnaryServerRateLimiterBlocking gRPC 服务端阻塞限流拦截器
func UnaryServerRateLimiterBlocking(limiter RateLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := limiter.Wait(ctx); err != nil {
			return nil, status.Error(codes.ResourceExhausted, "rate limit wait timeout")
		}
		return handler(ctx, req)
	}
}

// StreamClientCircuitBreaker gRPC 流式客户端熔断拦截器
func StreamClientCircuitBreaker(manager *CircuitBreakerManager) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		cb := manager.Get(method)

		if err := cb.Allow(); err != nil {
			return nil, status.Error(codes.Unavailable, "circuit breaker is open")
		}

		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			cb.RecordFailure()
			return nil, err
		}

		cb.RecordSuccess()
		return stream, nil
	}
}

// StreamServerRateLimiter gRPC 流式服务端限流拦截器
func StreamServerRateLimiter(limiter RateLimiter) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !limiter.Allow() {
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(srv, ss)
	}
}
