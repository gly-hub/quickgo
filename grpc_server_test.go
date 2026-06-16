package quickgo

import (
	"strings"
	"testing"

	"github.com/team-dandelion/quickgo/metrics"
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

func TestGrpcServerRegisterAddressPrefersExplicitValue(t *testing.T) {
	server, err := NewGrpcServer(&GrpcServerConfig{
		Address:         "0.0.0.0",
		Port:            50051,
		RegisterAddress: "10.0.0.12:50051",
	})
	if err != nil {
		t.Fatalf("NewGrpcServer failed: %v", err)
	}

	got, err := server.registerAddress()
	if err != nil {
		t.Fatalf("registerAddress failed: %v", err)
	}
	if got != "10.0.0.12:50051" {
		t.Fatalf("expected explicit register address, got %q", got)
	}
}

func TestGrpcServerRegisterAddressRequiresExplicitAddressForEtcdWildcardListen(t *testing.T) {
	t.Setenv("SERVER_IP", "")

	server, err := NewGrpcServer(&GrpcServerConfig{
		ServiceName: "svc",
		Address:     "0.0.0.0",
		Etcd:        &EtcdConfig{Endpoints: []string{"127.0.0.1:2379"}},
	})
	if err != nil {
		t.Fatalf("NewGrpcServer failed: %v", err)
	}

	if _, err := server.registerAddress(); err == nil || !strings.Contains(err.Error(), "registerAddress") {
		t.Fatalf("expected register address error, got %v", err)
	}
}

func TestGrpcServerClonesMetricsConfig(t *testing.T) {
	config := &GrpcServerConfig{
		Metrics: &metrics.Config{Namespace: "grpc", Buckets: []float64{0.1, 0.2}},
	}
	server, err := NewGrpcServer(config)
	if err != nil {
		t.Fatalf("NewGrpcServer failed: %v", err)
	}

	if server.config.Metrics == config.Metrics {
		t.Fatal("expected metrics config to be cloned")
	}
	config.Metrics.Buckets[0] = 9
	if server.config.Metrics.Buckets[0] == 9 {
		t.Fatal("expected metrics buckets to be cloned")
	}
}
