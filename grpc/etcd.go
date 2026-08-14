package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/gly-hub/quickgo/logger"
)

const (
	// DefaultEtcdPrefix 默认 etcd 前缀
	DefaultEtcdPrefix = "/grpc/services"
	// DefaultEtcdTTL 默认 TTL（秒）
	DefaultEtcdTTL             = 30
	etcdRecoveryInitialBackoff = time.Second
	etcdRecoveryMaxBackoff     = 30 * time.Second
)

var errNoServiceAddresses = errors.New("no addresses found")

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
	client     *clientv3.Client
	prefix     string
	key        string
	watchers   map[uint64]context.CancelFunc
	watcherSeq uint64
	mu         sync.RWMutex
	closed     bool
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
		key:      etcdConfigKey(config),
		watchers: make(map[uint64]context.CancelFunc),
	}, nil
}

// DiscoveryKey returns a stable key for enforcing one etcd config per resolver scheme.
func (r *EtcdResolver) DiscoveryKey() string {
	return r.key
}

// Resolve 解析服务地址
func (r *EtcdResolver) Resolve(ctx context.Context, serviceName string) ([]string, error) {
	key := path.Join(r.prefix, serviceName)

	r.mu.RLock()
	client := r.client
	closed := r.closed
	r.mu.RUnlock()
	if closed || client == nil {
		return nil, fmt.Errorf("etcd resolver is closed")
	}

	resp, err := client.Get(ctx, key, clientv3.WithPrefix())
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
		return nil, fmt.Errorf("%w for service: %s", errNoServiceAddresses, serviceName)
	}

	return addresses, nil
}

// Watch 监听服务变化
func (r *EtcdResolver) Watch(ctx context.Context, serviceName string, callback func([]string)) error {
	return r.watch(ctx, serviceName, callback, false)
}

// watchUntilDone waits until the underlying etcd watch ends. It preserves the
// asynchronous Watch API for existing callers while allowing the gRPC resolver
// to recreate a watch after compaction or a broken watch stream.
func (r *EtcdResolver) watchUntilDone(ctx context.Context, serviceName string, callback func([]string)) error {
	return r.watch(ctx, serviceName, callback, true)
}

func (r *EtcdResolver) watch(ctx context.Context, serviceName string, callback func([]string), wait bool) error {
	if callback == nil {
		return fmt.Errorf("etcd watch callback is nil")
	}

	key := path.Join(r.prefix, serviceName)

	r.mu.Lock()
	if r.closed || r.client == nil {
		r.mu.Unlock()
		return fmt.Errorf("etcd resolver is closed")
	}
	watchCtx, cancel := context.WithCancel(ctx)
	r.watcherSeq++
	watcherID := r.watcherSeq
	r.watchers[watcherID] = cancel
	client := r.client
	r.mu.Unlock()
	cleanup := func() {
		r.mu.Lock()
		delete(r.watchers, watcherID)
		r.mu.Unlock()
	}

	// 首次获取
	addresses, err := r.Resolve(watchCtx, serviceName)
	if err != nil {
		if !wait || !errors.Is(err, errNoServiceAddresses) {
			cancel()
			cleanup()
			return err
		}
		addresses = nil
	}
	callback(addresses)

	// 监听变化
	watchChan := client.Watch(watchCtx, key, clientv3.WithPrefix())
	done := make(chan error, 1)
	watchLoop := func() {
		defer cleanup()
		for {
			select {
			case <-watchCtx.Done():
				done <- watchCtx.Err()
				return
			case watchResp, ok := <-watchChan:
				if !ok {
					done <- fmt.Errorf("etcd watch channel closed for service: %s", serviceName)
					return
				}
				if watchResp.Canceled {
					if watchErr := watchResp.Err(); watchErr != nil {
						done <- fmt.Errorf("etcd watch canceled for service %s: %w", serviceName, watchErr)
					} else {
						done <- fmt.Errorf("etcd watch canceled for service: %s", serviceName)
					}
					return
				}

				// 重新解析服务地址
				addresses, err := r.Resolve(watchCtx, serviceName)
				if err != nil {
					if errors.Is(err, errNoServiceAddresses) {
						callback(nil)
						continue
					}
					done <- fmt.Errorf("resolve service after etcd watch event: %w", err)
					return
				}
				callback(addresses)
			}
		}
	}

	if wait {
		watchLoop()
		return <-done
	}
	go func() {
		if err := func() error {
			watchLoop()
			return <-done
		}(); err != nil && ctx.Err() == nil {
			logger.Warn(context.Background(), "Etcd watch ended: service=%s, error=%v", serviceName, err)
		}
	}()

	return nil
}

// Close 关闭服务发现
func (r *EtcdResolver) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true

	// 取消所有 watcher
	for _, cancel := range r.watchers {
		cancel()
	}
	r.watchers = make(map[uint64]context.CancelFunc)
	client := r.client
	r.client = nil
	r.mu.Unlock()

	if client != nil {
		return client.Close()
	}
	return nil
}

// EtcdRegistry etcd 服务注册实现
type EtcdRegistry struct {
	client          *clientv3.Client
	prefix          string
	ttl             int64
	leaseID         clientv3.LeaseID
	leaseKeep       <-chan *clientv3.LeaseKeepAliveResponse
	keepAliveCtx    context.Context
	keepAliveCancel context.CancelFunc
	registered      bool
	serviceName     string
	address         string
	metadata        map[string]string
	closed          bool
	mu              sync.RWMutex
	operationMu     sync.Mutex
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
	if serviceName == "" || address == "" {
		return errors.New("service name and address are required")
	}

	r.operationMu.Lock()
	defer r.operationMu.Unlock()

	r.mu.Lock()
	if r.closed || r.client == nil {
		r.mu.Unlock()
		return errors.New("etcd registry is closed")
	}
	r.stopKeepAliveLocked()
	r.registered = true
	r.serviceName = serviceName
	r.address = address
	r.metadata = cloneStringMap(metadata)
	r.mu.Unlock()

	if err := r.establishLease(ctx); err != nil {
		r.mu.Lock()
		r.registered = false
		r.mu.Unlock()
		return err
	}
	r.startKeepAlive()
	logger.Info(ctx, "Service registered to etcd: service=%s, address=%s, key=%s", serviceName, address, path.Join(r.prefix, serviceName, address))
	return nil
}

// Deregister 注销服务
func (r *EtcdRegistry) Deregister(ctx context.Context, serviceName, address string) error {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()

	r.mu.Lock()
	r.stopKeepAliveLocked()
	r.registered = false
	leaseID := r.leaseID
	r.leaseID = 0
	client := r.client
	r.mu.Unlock()

	// 撤销租约（会自动停止心跳）
	if leaseID != 0 && client != nil {
		_, err := client.Revoke(ctx, leaseID)
		if err != nil {
			logger.Error(ctx, "Failed to revoke lease: leaseID=%d, error=%v", leaseID, err)
		}
	}

	// 删除 key
	key := path.Join(r.prefix, serviceName, address)
	if client == nil {
		return errors.New("etcd registry is closed")
	}
	_, err := client.Delete(ctx, key)
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
	r.operationMu.Lock()
	defer r.operationMu.Unlock()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.registered = false
	r.stopKeepAliveLocked()
	leaseID := r.leaseID
	r.leaseID = 0
	client := r.client
	r.client = nil
	r.mu.Unlock()

	if client == nil {
		return nil
	}
	if leaseID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = client.Revoke(ctx, leaseID)
		cancel()
	}
	return client.Close()
}

func (r *EtcdRegistry) establishLease(ctx context.Context) error {
	r.mu.RLock()
	if r.closed || !r.registered || r.client == nil {
		r.mu.RUnlock()
		return errors.New("etcd registry is not active")
	}
	client := r.client
	ttl := r.ttl
	prefix := r.prefix
	serviceName := r.serviceName
	address := r.address
	metadata := cloneStringMap(r.metadata)
	r.mu.RUnlock()

	leaseResp, err := client.Grant(ctx, ttl)
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}
	leaseID := leaseResp.ID
	key := path.Join(prefix, serviceName, address)
	value := address
	if len(metadata) > 0 {
		metadataJSON, marshalErr := json.Marshal(metadata)
		if marshalErr != nil {
			_, _ = client.Revoke(ctx, leaseID)
			return fmt.Errorf("failed to marshal service metadata: %w", marshalErr)
		}
		value = string(metadataJSON)
	}
	if _, err := client.Put(ctx, key, value, clientv3.WithLease(leaseID)); err != nil {
		_, _ = client.Revoke(ctx, leaseID)
		return fmt.Errorf("failed to register service: %w", err)
	}

	keepAliveCtx, keepAliveCancel := context.WithCancel(context.Background())
	leaseKeep, err := client.KeepAlive(keepAliveCtx, leaseID)
	if err != nil {
		keepAliveCancel()
		_, _ = client.Revoke(ctx, leaseID)
		return fmt.Errorf("failed to start keepalive: %w", err)
	}

	r.mu.Lock()
	if r.closed || !r.registered || r.client != client {
		r.mu.Unlock()
		keepAliveCancel()
		_, _ = client.Revoke(context.Background(), leaseID)
		return errors.New("etcd registry stopped while registering service")
	}
	oldLeaseID := r.leaseID
	oldKeepAliveCancel := r.keepAliveCancel
	r.leaseID = leaseID
	r.leaseKeep = leaseKeep
	r.keepAliveCtx = keepAliveCtx
	r.keepAliveCancel = keepAliveCancel
	r.mu.Unlock()

	if oldKeepAliveCancel != nil {
		oldKeepAliveCancel()
	}
	if oldLeaseID != 0 && oldLeaseID != leaseID {
		_, _ = client.Revoke(context.Background(), oldLeaseID)
	}
	return nil
}

func (r *EtcdRegistry) startKeepAlive() {
	r.mu.RLock()
	if r.closed || !r.registered || r.keepAliveCancel == nil {
		r.mu.RUnlock()
		return
	}
	serviceName := r.serviceName
	address := r.address
	keepAliveCtx := r.keepAliveCtx
	r.mu.RUnlock()

	go r.keepAliveLoop(keepAliveCtx, serviceName, address)
}

func (r *EtcdRegistry) keepAliveLoop(ctx context.Context, serviceName, address string) {
	backoff := etcdRecoveryInitialBackoff
	for {
		r.mu.RLock()
		leaseKeep := r.leaseKeep
		currentKeepAliveCtx := r.keepAliveCtx
		registered := r.registered
		closed := r.closed
		r.mu.RUnlock()
		if closed || !registered || leaseKeep == nil || currentKeepAliveCtx != ctx {
			return
		}

		select {
		case <-ctx.Done():
			return
		case response, ok := <-leaseKeep:
			if ok && response != nil {
				continue
			}
		}

		logger.Warn(context.Background(), "Etcd keepalive stopped; re-registering service: service=%s, address=%s", serviceName, address)
		for {
			if ctx.Err() != nil {
				return
			}
			r.operationMu.Lock()
			recoveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := r.establishLease(recoveryCtx)
			cancel()
			r.operationMu.Unlock()
			if err == nil {
				r.startKeepAlive()
				return
			}
			logger.Warn(context.Background(), "Failed to re-register etcd service; retrying: service=%s, address=%s, error=%v", serviceName, address, err)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff = min(backoff*2, etcdRecoveryMaxBackoff)
		}
	}
}

func (r *EtcdRegistry) stopKeepAliveLocked() {
	if r.keepAliveCancel != nil {
		r.keepAliveCancel()
		r.keepAliveCancel = nil
	}
	r.leaseKeep = nil
	r.keepAliveCtx = nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// RegisterEtcdResolver 注册 etcd resolver
func RegisterEtcdResolver(config EtcdConfig) error {
	resolver, err := NewEtcdResolver(config)
	if err != nil {
		return err
	}
	return RegisterResolver(EtcdScheme, resolver)
}

func etcdConfigKey(config EtcdConfig) string {
	endpoints := append([]string(nil), config.Endpoints...)
	sort.Strings(endpoints)
	return fmt.Sprintf("endpoints=%s;dial=%s;prefix=%s;username=%s;password=%s",
		strings.Join(endpoints, ","),
		config.DialTimeout,
		config.Prefix,
		config.Username,
		config.Password,
	)
}
