package grpc

import (
	"strings"
	"testing"

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
