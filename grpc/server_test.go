package grpc

import (
	"net"
	"strings"
	"testing"
)

func TestServerStopAfterListenClosesListener(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := server.Listen(); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	addr := server.getListener().Addr().String()

	if err := server.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("expected listener address to be released after Stop, got %v", err)
	}
	_ = listener.Close()
}

func TestServerStartAsyncRejectsDuplicateStart(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() {
		if err := server.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	}()

	if err := server.StartAsync(); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	if err := server.StartAsync(); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected duplicate StartAsync to be rejected, got %v", err)
	}
}

func TestServerStopIsIdempotent(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := server.Stop(); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
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
