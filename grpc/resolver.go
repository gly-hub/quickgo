package grpc

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc/resolver"

	"gly-hub/go-dandelion/quickgo/logger"
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

// resolverBuilder gRPC resolver builder
type resolverBuilder struct {
	scheme string
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
	r := &serviceResolver{
		target: target,
		cc:     cc,
		sd:     b.sd,
		ctx:    context.Background(),
	}

	// 启动解析
	go r.start()

	return r, nil
}

// Scheme 返回 scheme
func (b *resolverBuilder) Scheme() string {
	return b.scheme
}

// serviceResolver gRPC resolver 实现
type serviceResolver struct {
	target resolver.Target
	cc     resolver.ClientConn
	sd     ServiceDiscovery
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// start 开始解析
func (r *serviceResolver) start() {
	r.mu.Lock()
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.mu.Unlock()

	serviceName := r.target.Endpoint()

	// 首次解析
	addresses, err := r.sd.Resolve(r.ctx, serviceName)
	if err != nil {
		logger.Error(r.ctx, "Failed to resolve service: service=%s", serviceName, err)
		return
	}

	r.updateState(addresses)

	// 监听服务变化
	go func() {
		err := r.sd.Watch(r.ctx, serviceName, func(addrs []string) {
			r.updateState(addrs)
		})
		if err != nil {
			logger.Error(r.ctx, "Service discovery watch failed: service=%s", serviceName, err)
		}
	}()
}

// updateState 更新连接状态
func (r *serviceResolver) updateState(addresses []string) {
	if len(addresses) == 0 {
		logger.Warn(r.ctx, "No addresses available for service: service=%s", r.target.Endpoint())
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
		logger.Error(r.ctx, "Failed to update resolver state: service=%s", r.target.Endpoint(), err)
		return
	}

	logger.Info(r.ctx, "Resolver state updated: service=%s, addresses=%v", r.target.Endpoint(), addresses)
}

// ResolveNow 立即重新解析
func (r *serviceResolver) ResolveNow(resolver.ResolveNowOptions) {
	serviceName := r.target.Endpoint()
	addresses, err := r.sd.Resolve(r.ctx, serviceName)
	if err != nil {
		logger.Error(r.ctx, "Failed to resolve service: service=%s", serviceName, err)
		return
	}
	r.updateState(addresses)
}

// Close 关闭 resolver
func (r *serviceResolver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
}

// RegisterResolver 注册 resolver
func RegisterResolver(scheme string, sd ServiceDiscovery) {
	builder := newResolverBuilder(scheme, sd)
	resolver.Register(builder)
	logger.Info(context.Background(), "Resolver registered: scheme=%s", scheme)
}

// RegisterStaticResolver 注册静态服务发现
func RegisterStaticResolver(addresses []string) {
	sd := NewStaticResolver(addresses)
	RegisterResolver(StaticScheme, sd)
}

