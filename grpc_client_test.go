package quickgo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	qgrpc "github.com/gly-hub/quickgo/grpc"
	"google.golang.org/grpc/connectivity"
)

func TestNewGrpcClientStaticDiscoveryRequiresAddress(t *testing.T) {
	_, err := NewGrpcClient("user-service", &GrpcClientConfig{
		Discovery:       "static",
		StaticAddresses: map[string]string{},
		Insecure:        true,
	})
	if err == nil || !strings.Contains(err.Error(), "static address is required") {
		t.Fatalf("expected static address error, got %v", err)
	}
}

func TestGrpcClientManagerStaticDiscoveryRequiresAddress(t *testing.T) {
	manager, err := NewGrpcClientManager(&GrpcClientConfig{
		Discovery:       "static",
		StaticAddresses: map[string]string{},
		Insecure:        true,
	})
	if err != nil {
		t.Fatalf("NewGrpcClientManager failed: %v", err)
	}
	if err := manager.RegisterService("user-service"); err != nil {
		t.Fatalf("RegisterService failed: %v", err)
	}

	_, err = manager.GetClient(t.Context(), "user-service")
	if err == nil || !strings.Contains(err.Error(), "static address is required") {
		t.Fatalf("expected static address error, got %v", err)
	}
}

func TestGrpcClientManagerRejectsInvalidDiscoveryConfig(t *testing.T) {
	testCases := []struct {
		name   string
		config *GrpcClientConfig
		want   string
	}{
		{
			name:   "unsupported discovery",
			config: &GrpcClientConfig{Discovery: "consul", Insecure: true},
			want:   "unsupported grpc client discovery",
		},
		{
			name:   "etcd without config",
			config: &GrpcClientConfig{Discovery: "etcd", Insecure: true},
			want:   "etcd config is required",
		},
		{
			name: "static with etcd",
			config: &GrpcClientConfig{
				Discovery: "static",
				Insecure:  true,
				Etcd:      &EtcdConfig{Endpoints: []string{"127.0.0.1:2379"}},
			},
			want: "must not be set",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewGrpcClientManager(testCase.config)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("expected error containing %q, got %v", testCase.want, err)
			}
		})
	}
}

func TestNormalizeGrpcClientDiscoveryInfersLegacyEtcdConfig(t *testing.T) {
	config := &GrpcClientConfig{Etcd: &EtcdConfig{Endpoints: []string{"127.0.0.1:2379"}}}
	if err := normalizeGrpcClientDiscovery(config); err != nil {
		t.Fatalf("normalizeGrpcClientDiscovery failed: %v", err)
	}
	if config.Discovery != qgrpc.EtcdScheme {
		t.Fatalf("expected legacy etcd config to infer %q, got %q", qgrpc.EtcdScheme, config.Discovery)
	}
}

func TestGrpcClientManagerHealthCheckDoesNotCloseCachedConn(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := qgrpc.NewServer(qgrpc.Config{
		Address: "127.0.0.1",
		Port:    port,
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if err := server.StartAsync(); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	manager, err := NewGrpcClientManager(&GrpcClientConfig{
		Discovery: "static",
		StaticAddresses: map[string]string{
			"ai-agent": fmt.Sprintf("127.0.0.1:%d", port),
		},
		Timeout:             "2s",
		Insecure:            true,
		HealthCheckInterval: "0",
	})
	if err != nil {
		t.Fatalf("NewGrpcClientManager failed: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.CloseAll()
	})
	if err := manager.RegisterService("ai-agent"); err != nil {
		t.Fatalf("RegisterService failed: %v", err)
	}

	conn, err := manager.GetConn(context.Background(), "ai-agent")
	if err != nil {
		t.Fatalf("GetConn failed: %v", err)
	}
	if !waitForConnState(t, conn, connectivity.Ready, time.Second) {
		t.Fatalf("connection did not become ready, got %s", conn.GetState())
	}

	if err := server.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if !waitForNotConnState(t, conn, connectivity.Ready, 2*time.Second) {
		t.Fatalf("connection stayed ready after server stop")
	}

	manager.performHealthCheck()

	if state := conn.GetState(); state == connectivity.Shutdown {
		t.Fatalf("cached ClientConn was closed by health check")
	}
}

func TestGrpcClientManagerGetClientFollowerHonorsOwnContext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	accepted := make(chan struct{})
	release := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		close(accepted)
		<-release
		_ = conn.Close()
	}()
	defer close(release)

	manager, err := NewGrpcClientManager(&GrpcClientConfig{
		Discovery: "static",
		StaticAddresses: map[string]string{
			"slow-service": listener.Addr().String(),
		},
		Timeout:             "500ms",
		Insecure:            true,
		HealthCheckInterval: "0",
	})
	if err != nil {
		t.Fatalf("NewGrpcClientManager failed: %v", err)
	}
	defer manager.CloseAll()
	if err := manager.RegisterService("slow-service"); err != nil {
		t.Fatalf("RegisterService failed: %v", err)
	}

	leaderDone := make(chan error, 1)
	go func() {
		_, leaderErr := manager.GetClient(context.Background(), "slow-service")
		leaderDone <- leaderErr
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("leader did not start connecting")
	}

	followerCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = manager.GetClient(followerCtx, "slow-service")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected follower deadline error, got %v", err)
	}

	<-leaderDone
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve tcp port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func waitForConnState(t *testing.T, conn interface {
	GetState() connectivity.State
	WaitForStateChange(context.Context, connectivity.State) bool
}, state connectivity.State, timeout time.Duration) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for conn.GetState() != state {
		if !conn.WaitForStateChange(ctx, conn.GetState()) {
			return false
		}
	}
	return true
}

func waitForNotConnState(t *testing.T, conn interface {
	GetState() connectivity.State
	WaitForStateChange(context.Context, connectivity.State) bool
}, state connectivity.State, timeout time.Duration) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for conn.GetState() == state {
		if !conn.WaitForStateChange(ctx, state) {
			return false
		}
	}
	return true
}
