package resilience

import (
	"context"
	"errors"
	"io"
	"sync"

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
			recordCircuitResult(cb, err)
			return nil, err
		}

		return &circuitBreakingClientStream{
			ClientStream:  stream,
			cb:            cb,
			serverStreams: desc != nil && desc.ServerStreams,
		}, nil
	}
}

type circuitBreakingClientStream struct {
	grpc.ClientStream
	cb            *CircuitBreaker
	serverStreams bool
	once          sync.Once
}

func (s *circuitBreakingClientStream) SendMsg(message interface{}) error {
	err := s.ClientStream.SendMsg(message)
	if err != nil {
		s.recordResult(err)
	}
	return err
}

func (s *circuitBreakingClientStream) RecvMsg(message interface{}) error {
	err := s.ClientStream.RecvMsg(message)
	if errors.Is(err, io.EOF) {
		s.recordSuccess()
	} else if err != nil {
		s.recordResult(err)
	} else if !s.serverStreams {
		// A client-streaming RPC returns its single terminal response without a
		// subsequent EOF read in generated CloseAndRecv helpers.
		s.recordSuccess()
	}
	return err
}

func (s *circuitBreakingClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.recordResult(err)
	}
	return err
}

func (s *circuitBreakingClientStream) recordSuccess() {
	s.once.Do(func() { s.cb.RecordSuccess() })
}

func (s *circuitBreakingClientStream) recordResult(err error) {
	s.once.Do(func() { recordCircuitResult(s.cb, err) })
}

func recordCircuitResult(cb *CircuitBreaker, err error) {
	if cb.isFailure(err) {
		cb.RecordFailure()
		return
	}
	cb.RecordSuccess()
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
