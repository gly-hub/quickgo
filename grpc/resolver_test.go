package grpc

import "testing"

func TestRegisterResolverAllowsSameConfig(t *testing.T) {
	scheme := "test-same-config"

	first := NewStaticResolver([]string{"127.0.0.1:9001", "127.0.0.1:9002"})
	second := NewStaticResolver([]string{"127.0.0.1:9002", "127.0.0.1:9001"})

	if err := RegisterResolver(scheme, first); err != nil {
		t.Fatalf("expected first resolver registration to succeed: %v", err)
	}
	if err := RegisterResolver(scheme, second); err != nil {
		t.Fatalf("expected same resolver config to be idempotent: %v", err)
	}
}

func TestRegisterResolverRejectsDifferentConfig(t *testing.T) {
	scheme := "test-different-config"

	first := NewStaticResolver([]string{"127.0.0.1:9001"})
	second := NewStaticResolver([]string{"127.0.0.1:9002"})

	if err := RegisterResolver(scheme, first); err != nil {
		t.Fatalf("expected first resolver registration to succeed: %v", err)
	}
	if err := RegisterResolver(scheme, second); err == nil {
		t.Fatal("expected different resolver config to be rejected")
	}
}
