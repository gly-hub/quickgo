package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"quickgo/logger"
)

const (
	// DefaultEtcdPrefix 默认 etcd 前缀
	DefaultEtcdPrefix = "/grpc/services"
	// DefaultEtcdTTL 默认 TTL（秒）
	DefaultEtcdTTL = 30
)

// EtcdConfig etcd 配置
type EtcdConfig struct {
	Endpoints   []string      // etcd 端点列表
	DialTimeout time.Duration // 连接超时
	Prefix      string        // 服务前缀，默认为 /grpc/services
	TTL         int64         // 租约 TTL（秒），默认为 30
	Username    string        // 用户名（可选）
	Password    string        // 密码（可选）
}

// EtcdResolver etcd 服务发现实现
type EtcdResolver struct {
	client   *clientv3.Client
	prefix   string
	watchers map[string]context.CancelFunc
	mu       sync.RWMutex
}

// NewEtcdResolver 创建 etcd 服务发现
func NewEtcdResolver(config EtcdConfig) (*EtcdResolver, error) {
	if len(config.Endpoints) == 0 {
		return nil, fmt.Errorf("etcd endpoints are required")
	}

	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}

	if config.Prefix == "" {
		config.Prefix = DefaultEtcdPrefix
	}

	etcdConfig := clientv3.Config{
		Endpoints:   config.Endpoints,
		DialTimeout: config.DialTimeout,
	}

	if config.Username != "" && config.Password != "" {
		etcdConfig.Username = config.Username
		etcdConfig.Password = config.Password
	}

	client, err := clientv3.New(etcdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &EtcdResolver{
		client:   client,
		prefix:   config.Prefix,
		watchers: make(map[string]context.CancelFunc),
	}, nil
}

// Resolve 解析服务地址
func (r *EtcdResolver) Resolve(ctx context.Context, serviceName string) ([]string, error) {
	key := path.Join(r.prefix, serviceName)

	resp, err := r.client.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get service from etcd: %w", err)
	}

	addresses := make([]string, 0, len(resp.Kvs))
	seen := make(map[string]bool)

	for _, kv := range resp.Kvs {
		// 从 key 中提取地址，格式：/prefix/service-name/address
		keyStr := string(kv.Key)
		parts := strings.Split(keyStr, "/")
		if len(parts) > 0 {
			addr := parts[len(parts)-1]
			if !seen[addr] {
				addresses = append(addresses, addr)
				seen[addr] = true
			}
		}
	}

	if len(addresses) == 0 {
		return nil, fmt.Errorf("no addresses found for service: %s", serviceName)
	}

	return addresses, nil
}

// Watch 监听服务变化
func (r *EtcdResolver) Watch(ctx context.Context, serviceName string, callback func([]string)) error {
	key := path.Join(r.prefix, serviceName)

	r.mu.Lock()
	// 如果已经有 watcher，先取消
	if cancel, ok := r.watchers[serviceName]; ok {
		cancel()
	}

	watchCtx, cancel := context.WithCancel(ctx)
	r.watchers[serviceName] = cancel
	r.mu.Unlock()

	// 首次获取
	addresses, err := r.Resolve(watchCtx, serviceName)
	if err == nil {
		callback(addresses)
	}

	// 监听变化
	watchChan := r.client.Watch(watchCtx, key, clientv3.WithPrefix())

	go func() {
		for {
			select {
			case <-watchCtx.Done():
				return
			case watchResp := <-watchChan:
				if watchResp.Canceled {
					return
				}

				// 重新解析服务地址
				addresses, err := r.Resolve(watchCtx, serviceName)
				if err == nil {
					callback(addresses)
				}
			}
		}
	}()

	return nil
}

// Close 关闭服务发现
func (r *EtcdResolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 取消所有 watcher
	for _, cancel := range r.watchers {
		cancel()
	}
	r.watchers = make(map[string]context.CancelFunc)

	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// EtcdRegistry etcd 服务注册实现
type EtcdRegistry struct {
	client    *clientv3.Client
	prefix    string
	ttl       int64
	leaseID   clientv3.LeaseID
	leaseKeep <-chan *clientv3.LeaseKeepAliveResponse
	mu        sync.RWMutex
}

// NewEtcdRegistry 创建 etcd 服务注册
func NewEtcdRegistry(config EtcdConfig) (*EtcdRegistry, error) {
	if len(config.Endpoints) == 0 {
		return nil, fmt.Errorf("etcd endpoints are required")
	}

	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}

	if config.Prefix == "" {
		config.Prefix = DefaultEtcdPrefix
	}

	if config.TTL == 0 {
		config.TTL = DefaultEtcdTTL
	}

	etcdConfig := clientv3.Config{
		Endpoints:   config.Endpoints,
		DialTimeout: config.DialTimeout,
	}

	if config.Username != "" && config.Password != "" {
		etcdConfig.Username = config.Username
		etcdConfig.Password = config.Password
	}

	client, err := clientv3.New(etcdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &EtcdRegistry{
		client: client,
		prefix: config.Prefix,
		ttl:    config.TTL,
	}, nil
}

// Register 注册服务
func (r *EtcdRegistry) Register(ctx context.Context, serviceName, address string, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 创建租约
	leaseResp, err := r.client.Grant(ctx, r.ttl)
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}
	r.leaseID = leaseResp.ID

	// 构建 key，格式：/prefix/service-name/address
	key := path.Join(r.prefix, serviceName, address)

	// 构建 value（包含元数据）
	value := address
	if len(metadata) > 0 {
		metadataJSON, err := json.Marshal(metadata)
		if err == nil {
			value = string(metadataJSON)
		}
	}

	// 注册服务
	_, err = r.client.Put(ctx, key, value, clientv3.WithLease(r.leaseID))
	if err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	// 启动心跳保持（使用独立的 context，因为心跳需要持续运行）
	keepAliveCtx := context.Background()
	r.leaseKeep, err = r.client.KeepAlive(keepAliveCtx, r.leaseID)
	if err != nil {
		return fmt.Errorf("failed to start keepalive: %w", err)
	}

	// 处理心跳响应
	go func() {
		for ka := range r.leaseKeep {
			if ka == nil {
				logger.Warn(keepAliveCtx, "KeepAlive channel closed: service=%s, address=%s", serviceName, address)
				return
			}
		}
	}()

	logger.Info(ctx, "Service registered to etcd: service=%s, address=%s, key=%s", serviceName, address, key)
	return nil
}

// Deregister 注销服务
func (r *EtcdRegistry) Deregister(ctx context.Context, serviceName, address string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 撤销租约（会自动停止心跳）
	if r.leaseID != 0 {
		_, err := r.client.Revoke(ctx, r.leaseID)
		if err != nil {
			logger.Error(ctx, "Failed to revoke lease: leaseID=%d", r.leaseID, err)
		}
		r.leaseID = 0
		r.leaseKeep = nil
	}

	// 删除 key
	key := path.Join(r.prefix, serviceName, address)
	_, err := r.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to deregister service: %w", err)
	}

	logger.Info(ctx, "Service deregistered from etcd: service=%s, address=%s", serviceName, address)
	return nil
}

// KeepAlive 保持服务活跃（心跳）
func (r *EtcdRegistry) KeepAlive(ctx context.Context, serviceName, address string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.leaseID == 0 {
		return fmt.Errorf("service not registered")
	}

	// 续约
	_, err := r.client.KeepAliveOnce(ctx, r.leaseID)
	if err != nil {
		return fmt.Errorf("failed to keepalive: %w", err)
	}

	return nil
}

// Close 关闭注册中心连接
func (r *EtcdRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 撤销租约（会自动停止心跳）
	if r.leaseID != 0 {
		ctx := context.Background()
		_, _ = r.client.Revoke(ctx, r.leaseID)
		r.leaseID = 0
		r.leaseKeep = nil
	}

	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// RegisterEtcdResolver 注册 etcd resolver
func RegisterEtcdResolver(config EtcdConfig) error {
	resolver, err := NewEtcdResolver(config)
	if err != nil {
		return err
	}
	RegisterResolver(EtcdScheme, resolver)
	return nil
}
