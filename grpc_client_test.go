package quickgo

import (
	"strings"
	"testing"
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
