package grpc

import (
	"context"
	"testing"

	grpcapi "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestClientAuthInterceptorPreservesOutgoingMetadata(t *testing.T) {
	interceptor := ClientAuthInterceptor("secret")
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"tenant", "tenant-1",
		"traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
	))

	err := interceptor(ctx, "/orders.Service/Create", nil, nil, nil, func(callCtx context.Context, _ string, _, _ interface{}, _ *grpcapi.ClientConn, _ ...grpcapi.CallOption) error {
		md, ok := metadata.FromOutgoingContext(callCtx)
		if !ok {
			t.Fatal("expected outgoing metadata")
		}
		if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer secret" {
			t.Fatalf("unexpected authorization metadata: %v", got)
		}
		if got := md.Get("tenant"); len(got) != 1 || got[0] != "tenant-1" {
			t.Fatalf("tenant metadata was dropped: %v", got)
		}
		if got := md.Get("traceparent"); len(got) != 1 || got[0] == "" {
			t.Fatalf("trace metadata was dropped: %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor failed: %v", err)
	}
}

func TestAuthInterceptorAcceptsOnlyExpectedToken(t *testing.T) {
	interceptor := AuthInterceptor("secret")
	handlerCalled := false
	handler := func(context.Context, interface{}) (interface{}, error) {
		handlerCalled = true
		return "ok", nil
	}
	info := &grpcapi.UnaryServerInfo{FullMethod: "/orders.Service/Create"}

	validCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret"))
	if _, err := interceptor(validCtx, nil, info, handler); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if !handlerCalled {
		t.Fatal("handler was not called for valid token")
	}

	invalidCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secrets"))
	if _, err := interceptor(invalidCtx, nil, info, handler); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated for invalid token, got %v", err)
	}
}
