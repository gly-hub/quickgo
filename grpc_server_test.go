package quickgo

import "testing"

func TestNewGrpcServerAppliesDefaults(t *testing.T) {
	config := &GrpcServerConfig{}

	server, err := NewGrpcServer(config)
	if err != nil {
		t.Fatalf("NewGrpcServer should accept minimal config: %v", err)
	}
	if server == nil {
		t.Fatal("NewGrpcServer returned nil server")
	}

	if config.Address != defaultGrpcServerAddress {
		t.Fatalf("expected default address %q, got %q", defaultGrpcServerAddress, config.Address)
	}
	if config.Port != defaultGrpcServerPort {
		t.Fatalf("expected default port %d, got %d", defaultGrpcServerPort, config.Port)
	}
}
