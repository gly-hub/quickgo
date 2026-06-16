package quickgo

import (
	"strings"
	"testing"
)

func TestNewGrpcServerAppliesDefaultsWithoutMutatingInput(t *testing.T) {
	config := &GrpcServerConfig{}

	server, err := NewGrpcServer(config)
	if err != nil {
		t.Fatalf("NewGrpcServer should accept minimal config: %v", err)
	}
	if server == nil {
		t.Fatal("NewGrpcServer returned nil server")
	}

	if config.Address != "" {
		t.Fatalf("expected input address to remain empty, got %q", config.Address)
	}
	if config.Port != 0 {
		t.Fatalf("expected input port to remain 0, got %d", config.Port)
	}
	if server.config.Address != defaultGrpcServerAddress {
		t.Fatalf("expected server default address %q, got %q", defaultGrpcServerAddress, server.config.Address)
	}
	if server.config.Port != defaultGrpcServerPort {
		t.Fatalf("expected server default port %d, got %d", defaultGrpcServerPort, server.config.Port)
	}
}

func TestNewGrpcServerValidatesEtcdConfig(t *testing.T) {
	_, err := NewGrpcServer(&GrpcServerConfig{
		Etcd: &EtcdConfig{Endpoints: []string{"127.0.0.1:2379"}},
	})
	if err == nil || !strings.Contains(err.Error(), "serviceName") {
		t.Fatalf("expected missing serviceName error, got %v", err)
	}

	_, err = NewGrpcServer(&GrpcServerConfig{
		ServiceName: "svc",
		Etcd:        &EtcdConfig{},
	})
	if err == nil || !strings.Contains(err.Error(), "endpoints") {
		t.Fatalf("expected missing endpoints error, got %v", err)
	}
}
