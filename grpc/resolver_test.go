package grpc

import (
	"context"
	"sync"
	"testing"
)

type closeCountingDiscovery struct {
	key    string
	closed int
}

func (d *closeCountingDiscovery) Resolve(ctx context.Context, serviceName string) ([]string, error) {
	return []string{"127.0.0.1:9001"}, nil
}

func (d *closeCountingDiscovery) Watch(ctx context.Context, serviceName string, callback func([]string)) error {
	callback([]string{"127.0.0.1:9001"})
	return nil
}

func (d *closeCountingDiscovery) Close() error {
	d.closed++
	return nil
}

func (d *closeCountingDiscovery) DiscoveryKey() string {
	return d.key
}

func TestRegisterResolverAllowsSameConfig(t *testing.T) {
	scheme := "test-same-config"

	first := NewStaticResolver([]string{"127.0.0.1:9001", "127.0.0.1:9002"})
	second := NewStaticResolver([]string{"127.0.0.1:9002", "127.0.0.1:9001"})

	if err := RegisterResolver(scheme, first); err != nil {
		t.Fatalf("expected first resolver registration to succeed: %v", err)
	}
	defer ReleaseResolver(scheme, first)
	if err := RegisterResolver(scheme, second); err != nil {
		t.Fatalf("expected same resolver config to be idempotent: %v", err)
	}
	defer ReleaseResolver(scheme, second)
}

func TestRegisterResolverRejectsDifferentConfig(t *testing.T) {
	scheme := "test-different-config"

	first := NewStaticResolver([]string{"127.0.0.1:9001"})
	second := NewStaticResolver([]string{"127.0.0.1:9002"})

	if err := RegisterResolver(scheme, first); err != nil {
		t.Fatalf("expected first resolver registration to succeed: %v", err)
	}
	defer ReleaseResolver(scheme, first)
	if err := RegisterResolver(scheme, second); err == nil {
		t.Fatal("expected different resolver config to be rejected")
	}
}

func TestRegisterResolverReferenceCountingClosesOnLastRelease(t *testing.T) {
	scheme := "test-ref-counting"
	first := &closeCountingDiscovery{key: "shared"}
	second := &closeCountingDiscovery{key: "shared"}

	registered, err := RegisterResolverAndGet(scheme, first)
	if err != nil {
		t.Fatalf("RegisterResolverAndGet(first) failed: %v", err)
	}
	if registered != first {
		t.Fatal("expected first registration to own resolver")
	}
	registered, err = RegisterResolverAndGet(scheme, second)
	if err != nil {
		t.Fatalf("RegisterResolverAndGet(second) failed: %v", err)
	}
	if registered != first {
		t.Fatal("expected second same-config registration to share first resolver")
	}

	if err := ReleaseResolver(scheme, first); err != nil {
		t.Fatalf("ReleaseResolver(first) failed: %v", err)
	}
	if first.closed != 0 {
		t.Fatalf("resolver closed before last release: %d", first.closed)
	}
	if err := ReleaseResolver(scheme, second); err != nil {
		t.Fatalf("ReleaseResolver(second) failed: %v", err)
	}
	if first.closed != 1 {
		t.Fatalf("expected resolver to close once on last release, got %d", first.closed)
	}

	third := &closeCountingDiscovery{key: "new-config"}
	registered, err = RegisterResolverAndGet(scheme, third)
	if err != nil {
		t.Fatalf("expected scheme to be reusable after release: %v", err)
	}
	if registered != third {
		t.Fatal("expected released scheme to bind to new resolver")
	}
	defer ReleaseResolver(scheme, third)
}

func TestClientConcurrentCloseAndReads(t *testing.T) {
	client, err := NewClient(ClientConfig{
		Address:  "127.0.0.1:1",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.GetConn()
			_ = client.IsConnected()
			_, _ = client.HealthCheck(context.Background(), "")
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = client.Close()
	}()
	wg.Wait()
}
