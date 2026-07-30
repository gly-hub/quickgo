package grpc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/resolver"
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

type resolverTestClientConn struct {
	resolver.ClientConn
	mu     sync.Mutex
	states []resolver.State
	errors []error
}

func (c *resolverTestClientConn) UpdateState(state resolver.State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states = append(c.states, state)
	return nil
}

func (c *resolverTestClientConn) ReportError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, err)
}

type blockingResolverClientConn struct {
	resolver.ClientConn
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	updates int
}

func (c *blockingResolverClientConn) UpdateState(resolver.State) error {
	select {
	case c.started <- struct{}{}:
	default:
	}
	<-c.release
	c.mu.Lock()
	c.updates++
	c.mu.Unlock()
	return nil
}

func (*blockingResolverClientConn) ReportError(error) {}

type blockingWatchDiscovery struct {
	watchStarted chan struct{}
}

func (d *blockingWatchDiscovery) Resolve(context.Context, string) ([]string, error) {
	return []string{"127.0.0.1:9001"}, nil
}

func (d *blockingWatchDiscovery) Watch(ctx context.Context, serviceName string, callback func([]string)) error {
	select {
	case d.watchStarted <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (d *blockingWatchDiscovery) Close() error {
	return nil
}

func TestServiceResolverCloseCancelsWatch(t *testing.T) {
	discovery := &blockingWatchDiscovery{watchStarted: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver := &serviceResolver{
		cc:          &resolverTestClientConn{},
		sd:          discovery,
		ctx:         ctx,
		cancel:      cancel,
		serviceName: "orders",
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		resolver.start()
	}()

	select {
	case <-discovery.watchStarted:
	case <-time.After(time.Second):
		t.Fatal("resolver did not start service watch")
	}
	resolver.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resolver start did not stop after Close")
	}
}

func TestServiceResolverCloseWaitsForStateUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cc := &blockingResolverClientConn{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	r := &serviceResolver{
		cc:          cc,
		ctx:         ctx,
		cancel:      cancel,
		serviceName: "orders",
	}

	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		r.updateState([]string{"127.0.0.1:9001"})
	}()

	select {
	case <-cc.started:
	case <-time.After(time.Second):
		t.Fatal("resolver did not start state update")
	}

	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		r.Close()
	}()

	select {
	case <-closeDone:
		t.Fatal("Close returned before the state update completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(cc.release)
	select {
	case <-updateDone:
	case <-time.After(time.Second):
		t.Fatal("state update did not complete")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after the state update")
	}

	r.updateState([]string{"127.0.0.1:9002"})
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.updates != 1 {
		t.Fatalf("expected one ClientConn update, got %d", cc.updates)
	}
}

func TestServiceResolverClearsStateWhenNoAddressesRemain(t *testing.T) {
	cc := &resolverTestClientConn{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolver := &serviceResolver{
		cc:          cc,
		ctx:         ctx,
		cancel:      cancel,
		serviceName: "orders",
	}

	resolver.updateState([]string{"127.0.0.1:9001"})
	resolver.updateState(nil)

	cc.mu.Lock()
	defer cc.mu.Unlock()
	if len(cc.states) != 2 {
		t.Fatalf("expected two resolver updates, got %d", len(cc.states))
	}
	if len(cc.states[1].Addresses) != 0 {
		t.Fatalf("expected empty resolver state, got %#v", cc.states[1].Addresses)
	}
}

type restartingWatchDiscovery struct {
	mu          sync.Mutex
	watchCalls  int
	secondWatch chan struct{}
}

func (d *restartingWatchDiscovery) Resolve(context.Context, string) ([]string, error) {
	return []string{"127.0.0.1:9001"}, nil
}

func (d *restartingWatchDiscovery) Watch(context.Context, string, func([]string)) error {
	return errors.New("unexpected asynchronous watch call")
}

func (d *restartingWatchDiscovery) watchUntilDone(ctx context.Context, _ string, callback func([]string)) error {
	d.mu.Lock()
	d.watchCalls++
	call := d.watchCalls
	d.mu.Unlock()

	if call == 1 {
		return errors.New("watch terminated")
	}
	callback([]string{"127.0.0.1:9002"})
	select {
	case d.secondWatch <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (d *restartingWatchDiscovery) Close() error { return nil }

func TestServiceResolverRestartsLifecycleWatch(t *testing.T) {
	cc := &resolverTestClientConn{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	discovery := &restartingWatchDiscovery{secondWatch: make(chan struct{}, 1)}
	r := &serviceResolver{
		cc:          cc,
		sd:          discovery,
		ctx:         ctx,
		cancel:      cancel,
		serviceName: "orders",
		retryDelay:  time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		r.start()
	}()

	select {
	case <-discovery.secondWatch:
	case <-time.After(time.Second):
		t.Fatal("resolver did not restart its lifecycle watch")
	}
	r.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resolver did not stop after Close")
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()
	if len(cc.errors) == 0 {
		t.Fatal("expected terminated watch to be reported")
	}
	if len(cc.states) == 0 || len(cc.states[len(cc.states)-1].Addresses) != 1 || cc.states[len(cc.states)-1].Addresses[0].Addr != "127.0.0.1:9002" {
		t.Fatalf("expected restarted watch address update, got %#v", cc.states)
	}
}

type emptyAddressWatchDiscovery struct {
	watchStarted chan struct{}
}

func (d *emptyAddressWatchDiscovery) Resolve(context.Context, string) ([]string, error) {
	return []string{"127.0.0.1:9001"}, nil
}

func (d *emptyAddressWatchDiscovery) Watch(context.Context, string, func([]string)) error {
	return errors.New("unexpected asynchronous watch call")
}

func (d *emptyAddressWatchDiscovery) watchUntilDone(ctx context.Context, _ string, callback func([]string)) error {
	callback(nil)
	select {
	case d.watchStarted <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (d *emptyAddressWatchDiscovery) Close() error { return nil }

func TestServiceResolverClearsStateAfterLifecycleWatchReportsNoAddresses(t *testing.T) {
	cc := &resolverTestClientConn{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &serviceResolver{
		cc:          cc,
		sd:          &emptyAddressWatchDiscovery{watchStarted: make(chan struct{}, 1)},
		ctx:         ctx,
		cancel:      cancel,
		serviceName: "orders",
	}
	discovery := r.sd.(*emptyAddressWatchDiscovery)

	done := make(chan struct{})
	go func() {
		defer close(done)
		r.start()
	}()
	select {
	case <-discovery.watchStarted:
	case <-time.After(time.Second):
		t.Fatal("resolver did not receive the empty address update")
	}
	r.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resolver did not stop after Close")
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()
	if len(cc.states) == 0 || len(cc.states[len(cc.states)-1].Addresses) != 0 {
		t.Fatalf("expected resolver state to be cleared, got %#v", cc.states)
	}
}

func TestEtcdResolverTracksConcurrentWatchersIndependently(t *testing.T) {
	r := &EtcdResolver{watchers: make(map[uint64]context.CancelFunc)}
	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer firstCancel()
	defer secondCancel()

	r.mu.Lock()
	r.watcherSeq++
	firstID := r.watcherSeq
	r.watchers[firstID] = firstCancel
	r.watcherSeq++
	secondID := r.watcherSeq
	r.watchers[secondID] = secondCancel
	r.mu.Unlock()

	select {
	case <-firstCtx.Done():
		t.Fatal("first watcher was canceled by registering a second watcher")
	default:
	}
	select {
	case <-secondCtx.Done():
		t.Fatal("second watcher was canceled unexpectedly")
	default:
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	for name, watchCtx := range map[string]context.Context{"first": firstCtx, "second": secondCtx} {
		select {
		case <-watchCtx.Done():
		case <-time.After(time.Second):
			t.Fatalf("%s watcher was not canceled by Close", name)
		}
	}
}
