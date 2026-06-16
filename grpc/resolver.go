package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

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
	target      resolver.Target
	cc          resolver.ClientConn
	sd          ServiceDiscovery
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	serviceName string // 缓存解析后的服务名
}

// getServiceName 从 target 中解析服务名（兼容新旧版本 gRPC）
func (r *serviceResolver) getServiceName() string {
	if r.serviceName != "" {
		return r.serviceName
	}

	// 尝试从 Endpoint() 获取（旧版本 gRPC）
	serviceName := r.target.Endpoint()
	if serviceName == "" {
		// 新版本 gRPC 中 Endpoint() 可能返回空，需要从 URL 中获取
		// 格式: etcd://service-name 或 etcd:///service-name
		if r.target.URL.Host != "" {
			serviceName = r.target.URL.Host
		} else if r.target.URL.Opaque != "" {
			serviceName = r.target.URL.Opaque
		} else {
			// 移除开头的 /
			serviceName = strings.TrimPrefix(r.target.URL.Path, "/")
		}
	}

	r.serviceName = serviceName
	return serviceName
}

// start 开始解析
func (r *serviceResolver) start() {
	r.mu.Lock()
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.mu.Unlock()

	serviceName := r.getServiceName()
	if serviceName == "" {
		logger.Error(r.ctx, "Failed to parse service name from target: %v", r.target)
		return
	}

	logger.Info(r.ctx, "Resolver starting for service: %s", serviceName)

	// 首次解析
	addresses, err := r.sd.Resolve(r.ctx, serviceName)
	if err != nil {
		logger.Error(r.ctx, "Failed to resolve service: service=%s, error=%v", serviceName, err)
		return
	}

	r.updateState(addresses)

	// 监听服务变化
	go func() {
		err := r.sd.Watch(r.ctx, serviceName, func(addrs []string) {
			r.updateState(addrs)
		})
		if err != nil {
			logger.Error(r.ctx, "Service discovery watch failed: service=%s, error=%v", serviceName, err)
		}
	}()
}

// updateState 更新连接状态
func (r *serviceResolver) updateState(addresses []string) {
	serviceName := r.getServiceName()
	if len(addresses) == 0 {
		logger.Warn(r.ctx, "No addresses available for service: service=%s", serviceName)
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
}

type registeredResolver struct {
	key string
	sd  ServiceDiscovery
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
		if registered.key != key {
			return nil, fmt.Errorf("resolver scheme %s already registered with a different config", scheme)
		}
		logger.Debug(context.Background(), "Resolver already registered, skipping: scheme=%s", scheme)
		return registered.sd, nil
	}

	builder := newResolverBuilder(scheme, sd)
	resolver.Register(builder)
	registeredSchemes[scheme] = registeredResolver{key: key, sd: sd}
	logger.Info(context.Background(), "Resolver registered: scheme=%s", scheme)
	return sd, nil
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
