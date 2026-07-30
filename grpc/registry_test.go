package grpc

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStaticRegistryDoesNotExposeMetadata(t *testing.T) {
	registry := NewStaticRegistry()
	metadata := map[string]string{"weight": "3", "region": "cn"}
	if err := registry.Register(context.Background(), "orders", "127.0.0.1:9001", metadata); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	metadata["region"] = "mutated"

	services := registry.GetServices("orders")
	if len(services) != 1 || services[0].Metadata["region"] != "cn" || services[0].Weight != 3 {
		t.Fatalf("unexpected registered service: %#v", services)
	}
	services[0].Metadata["region"] = "mutated-again"
	services = registry.GetServices("orders")
	if got := services[0].Metadata["region"]; got != "cn" {
		t.Fatalf("mutating returned service changed registry: %q", got)
	}
}

type recordingRegistry struct {
	mu        sync.Mutex
	metadata  map[string]string
	keepAlive chan struct{}
}

func (r *recordingRegistry) Register(_ context.Context, _ string, _ string, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadata = cloneMetadata(metadata)
	return nil
}

func (r *recordingRegistry) Deregister(context.Context, string, string) error { return nil }

func (r *recordingRegistry) KeepAlive(context.Context, string, string) error {
	select {
	case r.keepAlive <- struct{}{}:
	default:
	}
	return nil
}

func (r *recordingRegistry) Close() error { return nil }

func TestServiceRegistrarCopiesMetadataAndStartsOnlyOneKeepAlive(t *testing.T) {
	registry := &recordingRegistry{keepAlive: make(chan struct{}, 1)}
	metadata := map[string]string{"region": "cn"}
	registrar := NewServiceRegistrar(registry, "orders", "127.0.0.1:9001", metadata)
	defer registrar.Close()
	metadata["region"] = "mutated"

	if err := registrar.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	registry.mu.Lock()
	registeredMetadata := cloneMetadata(registry.metadata)
	registry.mu.Unlock()
	if got := registeredMetadata["region"]; got != "cn" {
		t.Fatalf("mutated constructor metadata reached registry: %q", got)
	}

	registrar.StartKeepAlive(time.Millisecond)
	registrar.mu.Lock()
	ticker := registrar.keepAliveTicker
	registrar.mu.Unlock()
	registrar.StartKeepAlive(time.Millisecond)
	select {
	case <-registry.keepAlive:
	case <-time.After(time.Second):
		t.Fatal("expected keepalive call")
	}

	if ticker == nil {
		t.Fatal("expected active keepalive ticker")
	}
	registrar.mu.Lock()
	currentTicker := registrar.keepAliveTicker
	registrar.mu.Unlock()
	if currentTicker != ticker {
		t.Fatal("StartKeepAlive replaced an active ticker")
	}
	if err := registrar.Deregister(context.Background()); err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}
}

type blockingKeepAliveRegistry struct {
	started chan struct{}
	exited  chan struct{}
}

func (r *blockingKeepAliveRegistry) Register(context.Context, string, string, map[string]string) error {
	return nil
}

func (r *blockingKeepAliveRegistry) Deregister(context.Context, string, string) error { return nil }

func (r *blockingKeepAliveRegistry) KeepAlive(ctx context.Context, _ string, _ string) error {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	select {
	case r.exited <- struct{}{}:
	default:
	}
	return ctx.Err()
}

func (r *blockingKeepAliveRegistry) Close() error { return nil }

func TestServiceRegistrarDeregisterCancelsKeepAlive(t *testing.T) {
	registry := &blockingKeepAliveRegistry{
		started: make(chan struct{}, 1),
		exited:  make(chan struct{}, 1),
	}
	registrar := NewServiceRegistrar(registry, "orders", "127.0.0.1:9001", nil)
	defer registrar.Close()
	registrar.StartKeepAlive(time.Millisecond)
	select {
	case <-registry.started:
	case <-time.After(time.Second):
		t.Fatal("expected keepalive call")
	}
	if err := registrar.Deregister(context.Background()); err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}
	select {
	case <-registry.exited:
	case <-time.After(time.Second):
		t.Fatal("Deregister did not cancel the in-flight keepalive")
	}
}
