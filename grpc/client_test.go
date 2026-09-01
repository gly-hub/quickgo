package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	rpc "google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func TestClientIsUsableAllowsRecoverableStates(t *testing.T) {
	conn, err := rpc.NewClient("passthrough:///127.0.0.1:1", rpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	client := &Client{conn: conn}
	if client.IsConnected() {
		t.Fatalf("new grpc ClientConn should not be ready before dialing")
	}
	if !client.IsUsable() {
		t.Fatalf("idle grpc ClientConn should be usable")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if state := conn.GetState(); state != connectivity.Shutdown {
		t.Fatalf("expected shutdown state, got %s", state)
	}
	if client.IsUsable() {
		t.Fatalf("shutdown grpc ClientConn should not be usable")
	}
}

func TestNewClientRequiresExplicitTransportSecurity(t *testing.T) {
	_, err := NewClient(ClientConfig{Address: "127.0.0.1:50051"})
	if err == nil || !strings.Contains(err.Error(), "TLS config is required") {
		t.Fatalf("expected fail-closed TLS error, got %v", err)
	}
}

func TestNewClientRejectsPartialMutualTLSConfig(t *testing.T) {
	_, err := NewClient(ClientConfig{
		Address: "127.0.0.1:50051",
		TLS:     &TLSConfig{CertFile: "client.pem"},
	})
	if err == nil || !strings.Contains(err.Error(), "both certFile and keyFile") {
		t.Fatalf("expected partial mutual TLS error, got %v", err)
	}
}

func TestWaitForReadyUnaryInterceptorAddsDeadlineWhenMissing(t *testing.T) {
	interceptor := waitForReadyUnaryInterceptor(50 * time.Millisecond)
	var got context.Context
	err := interceptor(context.Background(), "/test.Service/Call", nil, nil, nil, func(ctx context.Context, _ string, _ interface{}, _ interface{}, _ *rpc.ClientConn, _ ...rpc.CallOption) error {
		got = ctx
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if _, ok := got.Deadline(); !ok {
		t.Fatal("expected interceptor to add a deadline")
	}
}

func TestWaitForReadyUnaryInterceptorPreservesCallerDeadline(t *testing.T) {
	interceptor := waitForReadyUnaryInterceptor(time.Second)
	callerCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	callerDeadline, _ := callerCtx.Deadline()

	var got context.Context
	err := interceptor(callerCtx, "/test.Service/Call", nil, nil, nil, func(ctx context.Context, _ string, _ interface{}, _ interface{}, _ *rpc.ClientConn, _ ...rpc.CallOption) error {
		got = ctx
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	gotDeadline, ok := got.Deadline()
	if !ok || !gotDeadline.Equal(callerDeadline) {
		t.Fatalf("expected caller deadline %v, got %v", callerDeadline, gotDeadline)
	}
}
