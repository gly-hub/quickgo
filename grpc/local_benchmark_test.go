package grpc_test

import (
	"context"
	"net"
	"testing"

	"github.com/gly-hub/quickgo/metrics"
	"github.com/gly-hub/quickgo/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const benchmarkBufSize = 1024 * 1024

type benchmarkService interface {
	Ping(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

type benchmarkServer struct{}

func (benchmarkServer) Ping(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

var benchmarkServiceDesc = grpc.ServiceDesc{
	ServiceName: "quickgo.benchmark.Bench",
	HandlerType: (*benchmarkService)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "Ping",
		Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
			in := new(emptypb.Empty)
			if err := dec(in); err != nil {
				return nil, err
			}
			if interceptor == nil {
				return srv.(benchmarkService).Ping(ctx, in)
			}
			info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/quickgo.benchmark.Bench/Ping"}
			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return srv.(benchmarkService).Ping(ctx, req.(*emptypb.Empty))
			}
			return interceptor(ctx, in, info, handler)
		},
	}},
}

func BenchmarkLocalGRPC(b *testing.B) {
	b.Run("baseline", func(b *testing.B) {
		benchmarkGRPCCalls(b)
	})
	b.Run("metrics", func(b *testing.B) {
		m := metrics.New(metrics.Config{Namespace: "grpc_benchmark"})
		benchmarkGRPCCalls(b, metrics.UnaryServerInterceptor(m))
	})
	b.Run("tracing", func(b *testing.B) {
		benchmarkGRPCCalls(b, tracing.UnaryServerInterceptor())
	})
	b.Run("metrics_tracing", func(b *testing.B) {
		m := metrics.New(metrics.Config{Namespace: "grpc_trace_benchmark"})
		benchmarkGRPCCalls(b, metrics.UnaryServerInterceptor(m), tracing.UnaryServerInterceptor())
	})
}

func benchmarkGRPCCalls(b *testing.B, interceptors ...grpc.UnaryServerInterceptor) {
	b.Helper()
	listener := bufconn.Listen(benchmarkBufSize)
	options := make([]grpc.ServerOption, 0, 1)
	if len(interceptors) > 0 {
		options = append(options, grpc.ChainUnaryInterceptor(interceptors...))
	}
	server := grpc.NewServer(options...)
	server.RegisterService(&benchmarkServiceDesc, benchmarkServer{})
	go func() { _ = server.Serve(listener) }()

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		server.Stop()
		listener.Close()
		b.Fatal(err)
	}
	defer func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	}()

	request := &emptypb.Empty{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response := new(emptypb.Empty)
		if err := conn.Invoke(context.Background(), "/quickgo.benchmark.Bench/Ping", request, response); err != nil {
			b.Fatal(err)
		}
	}
}
