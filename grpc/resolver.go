package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/resolver"

	"github.com/team-dandelion/quickgo/logger"
)

const (
	// StaticScheme 静态服务发现方案
	StaticScheme = "static"
	// DNSScheme DNS服务发现方案
	DNSScheme = "dns"
	// EtcdScheme etcd 服务发现方案
	EtcdScheme = "etcd"
)

// ServiceDiscovery 服务发现接口
type ServiceDiscovery interface {
	// Resolve 解析服务地址
	Resolve(ctx context.Context, serviceName string) ([]string, error)
	// Watch 监听服务变化
	Watch(ctx context.Context, serviceName string, callback func([]string)) error
	// Close 关闭服务发现
	Close() error
}

type discoveryKeyer interface {
	DiscoveryKey() string
}

// watchUntilDoneDiscovery is implemented by discoveries whose Watch method is
// intentionally asynchronous. It lets the gRPC resolver observe a terminated
// watch and establish a new one without changing the public Watch contract.
type watchUntilDoneDiscovery interface {
	watchUntilDone(ctx context.Context, serviceName string, callback func([]string)) error
}

// StaticResolver 静态服务发现（直接指定地址列表）
type StaticResolver struct {
	addresses []string
	mu        sync.RWMutex
}

// NewStaticResolver 创建静态服务发现
func NewStaticResolver(addresses []string) *StaticResolver {
	return &StaticResolver{
		addresses: addresses,
	}
}

// Resolve 解析服务地址
func (r *StaticResolver) Resolve(ctx context.Context, serviceName string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.addresses) == 0 {
		return nil, fmt.Errorf("no addresses available")
	}

	// 返回地址的副本
	result := make([]string, len(r.addresses))
	copy(result, r.addresses)
	return result, nil
}

// Watch 监听服务变化（静态服务发现不需要监听）
func (r *StaticResolver) Watch(ctx context.Context, serviceName string, callback func([]string)) error {
	// 静态服务发现不需要监听，直接调用一次回调
	addresses, err := r.Resolve(ctx, serviceName)
	if err != nil {
		return err
	}
	callback(addresses)
	return nil
}

// Close 关闭服务发现
func (r *StaticResolver) Close() error {
	return nil
}

// UpdateAddresses 更新地址列表
func (r *StaticResolver) UpdateAddresses(addresses []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addresses = addresses
}

// DiscoveryKey returns a stable key for enforcing one config per resolver scheme.
func (r *StaticResolver) DiscoveryKey() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	addresses := append([]string(nil), r.addresses...)
	sort.Strings(addresses)
	return "static:" + strings.Join(addresses, ",")
}

// resolverBuilder gRPC resolver builder
type resolverBuilder struct {
	scheme string
	mu     sync.RWMutex
	sd     ServiceDiscovery
}

// newResolverBuilder 创建新的 resolver builder
func newResolverBuilder(scheme string, sd ServiceDiscovery) *resolverBuilder {
	return &resolverBuilder{
		scheme: scheme,
		sd:     sd,
	}
}

// Build 构建 resolver
func (b *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	b.mu.RLock()
	sd := b.sd
	b.mu.RUnlock()
	if sd == nil {
		return nil, fmt.Errorf("resolver scheme %s is not active", b.scheme)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &serviceResolver{
		target:      target,
		cc:          cc,
		sd:          sd,
		ctx:         ctx,
		cancel:      cancel,
		serviceName: serviceNameFromTarget(target),
	}

	// 启动解析
	go r.start()

	return r, nil
}

// Scheme 返回 scheme
func (b *resolverBuilder) Scheme() string {
	return b.scheme
}

func (b *resolverBuilder) setDiscovery(sd ServiceDiscovery) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sd = sd
}

// serviceResolver gRPC resolver 实现
type serviceResolver struct {
	target      resolver.Target
	cc          resolver.ClientConn
	sd          ServiceDiscovery
	ctx         context.Context
	cancel      context.CancelFunc
	serviceName string // 缓存解析后的服务名
	stateMu     sync.Mutex
	closeOnce   sync.Once
	retryDelay  time.Duration
}

// getServiceName 从 target 中解析服务名（兼容新旧版本 gRPC）
func (r *serviceResolver) getServiceName() string {
	return r.serviceName
}

func serviceNameFromTarget(target resolver.Target) string {
	// 尝试从 Endpoint() 获取（旧版本 gRPC）
	serviceName := target.Endpoint()
	if serviceName == "" {
		// 新版本 gRPC 中 Endpoint() 可能返回空，需要从 URL 中获取
		// 格式: etcd://service-name 或 etcd:///service-name
		if target.URL.Host != "" {
			serviceName = target.URL.Host
		} else if target.URL.Opaque != "" {
			serviceName = target.URL.Opaque
		} else {
			// 移除开头的 /
			serviceName = strings.TrimPrefix(target.URL.Path, "/")
		}
	}
	return serviceName
}

// start 开始解析
func (r *serviceResolver) start() {
	ctx := r.ctx
	if ctx == nil || ctx.Err() != nil {
		return
	}

	serviceName := r.getServiceName()
	if serviceName == "" {
		logger.Error(ctx, "Failed to parse service name from target: %v", r.target)
		return
	}

	logger.Info(r.ctx, "Resolver starting for service: %s", serviceName)

	// 首次解析
	addresses, err := r.sd.Resolve(ctx, serviceName)
	if err != nil {
		logger.Error(r.ctx, "Failed to resolve service: service=%s, error=%v", serviceName, err)
		r.cc.ReportError(err)
	} else {
		r.updateState(addresses)
	}

	watch := r.sd.Watch
	restartWatch := false
	if lifecycleWatcher, ok := r.sd.(watchUntilDoneDiscovery); ok {
		watch = lifecycleWatcher.watchUntilDone
		restartWatch = true
	}

	for {
		err := watch(ctx, serviceName, func(addrs []string) {
			r.updateState(addrs)
		})
		if ctx.Err() != nil {
			return
		}
		if err == nil && !restartWatch {
			return
		}
		if err == nil {
			logger.Warn(ctx, "Service discovery watch ended unexpectedly: service=%s", serviceName)
		} else {
			logger.Error(ctx, "Service discovery watch failed: service=%s, error=%v", serviceName, err)
			r.cc.ReportError(err)
		}

		delay := r.retryDelay
		if delay <= 0 {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if addresses, resolveErr := r.sd.Resolve(ctx, serviceName); resolveErr == nil {
			r.updateState(addresses)
		}
	}
}

// updateState 更新连接状态
func (r *serviceResolver) updateState(addresses []string) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if r.ctx == nil || r.ctx.Err() != nil {
		return
	}

	serviceName := r.getServiceName()
	if len(addresses) == 0 {
		logger.Warn(r.ctx, "No addresses available for service: service=%s", serviceName)
		if err := r.cc.UpdateState(resolver.State{}); err != nil {
			logger.Error(r.ctx, "Failed to clear resolver state: service=%s, error=%v", serviceName, err)
		}
		return
	}

	addrs := make([]resolver.Address, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, resolver.Address{
			Addr: addr,
		})
	}

	state := resolver.State{
		Addresses: addrs,
	}

	if err := r.cc.UpdateState(state); err != nil {
		logger.Error(r.ctx, "Failed to update resolver state: service=%s, error=%v", serviceName, err)
		return
	}

	logger.Info(r.ctx, "Resolver state updated: service=%s, addresses=%v", serviceName, addresses)
}

// ResolveNow 立即重新解析
func (r *serviceResolver) ResolveNow(resolver.ResolveNowOptions) {
	if r.ctx == nil || r.ctx.Err() != nil {
		return
	}
	serviceName := r.getServiceName()
	if serviceName == "" {
		return
	}
	addresses, err := r.sd.Resolve(r.ctx, serviceName)
	if err != nil {
		logger.Error(r.ctx, "Failed to resolve service: service=%s, error=%v", serviceName, err)
		return
	}
	r.updateState(addresses)
}

// Close 关闭 resolver
func (r *serviceResolver) Close() {
	r.closeOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		// Wait for an in-flight state update so Close does not return while a
		// callback can still update the underlying ClientConn.
		r.stateMu.Lock()
		r.stateMu.Unlock()
	})
}

type registeredResolver struct {
	key     string
	sd      ServiceDiscovery
	builder *resolverBuilder
	refs    int
}

// registeredSchemes 记录已注册的 scheme 配置，避免同一 scheme 混用不同配置。
var registeredSchemes = make(map[string]registeredResolver)
var registeredSchemesMu sync.Mutex

// RegisterResolver 注册 resolver（幂等，同一 scheme 只注册一次）
func RegisterResolver(scheme string, sd ServiceDiscovery) error {
	_, err := RegisterResolverAndGet(scheme, sd)
	return err
}

// RegisterResolverAndGet 注册 resolver，并返回该 scheme 实际使用的全局 resolver。
func RegisterResolverAndGet(scheme string, sd ServiceDiscovery) (ServiceDiscovery, error) {
	registeredSchemesMu.Lock()
	defer registeredSchemesMu.Unlock()

	key := resolverConfigKey(sd)

	// 检查是否已经注册过
	if registered, ok := registeredSchemes[scheme]; ok {
		if registered.refs > 0 && registered.key != key {
			return nil, fmt.Errorf("resolver scheme %s already registered with a different config", scheme)
		}
		if registered.refs == 0 {
			registered.key = key
			registered.sd = sd
			registered.builder.setDiscovery(sd)
		}
		registered.refs++
		registeredSchemes[scheme] = registered
		logger.Debug(context.Background(), "Resolver already registered, skipping: scheme=%s", scheme)
		return registered.sd, nil
	}

	builder := newResolverBuilder(scheme, sd)
	resolver.Register(builder)
	registeredSchemes[scheme] = registeredResolver{key: key, sd: sd, builder: builder, refs: 1}
	logger.Info(context.Background(), "Resolver registered: scheme=%s", scheme)
	return sd, nil
}

// ReleaseResolver releases one registration reference for a scheme/config pair.
// gRPC resolver builders are process-global and cannot be unregistered, so the
// scheme entry remains, but the underlying discovery is closed when no clients
// still reference it.
func ReleaseResolver(scheme string, sd ServiceDiscovery) error {
	registeredSchemesMu.Lock()
	defer registeredSchemesMu.Unlock()

	registered, ok := registeredSchemes[scheme]
	if !ok {
		return nil
	}
	if registered.sd != sd && registered.key != resolverConfigKey(sd) {
		return nil
	}
	if registered.refs > 0 {
		registered.refs--
	}
	if registered.refs == 0 && registered.sd != nil {
		if err := registered.sd.Close(); err != nil {
			return err
		}
		registered.sd = nil
		registered.builder.setDiscovery(nil)
	}
	registeredSchemes[scheme] = registered
	return nil
}

// RegisterStaticResolver 注册静态服务发现
func RegisterStaticResolver(addresses []string) error {
	sd := NewStaticResolver(addresses)
	return RegisterResolver(StaticScheme, sd)
}

func resolverConfigKey(sd ServiceDiscovery) string {
	if keyer, ok := sd.(discoveryKeyer); ok {
		sum := sha256.Sum256([]byte(keyer.DiscoveryKey()))
		return hex.EncodeToString(sum[:])
	}
	return fmt.Sprintf("%T:%p", sd, sd)
}
