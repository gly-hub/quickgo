package quickgo

import (
	"context"
	"errors"
	"fmt"

	"github.com/team-dandelion/quickgo/grpc"
	"github.com/team-dandelion/quickgo/logger"
	"sync"
	"sync/atomic"
	"time"

	rpc "google.golang.org/grpc"
)

// GrpcClientConfig gRPC 客户端配置（全局配置，所有服务共享）
type GrpcClientConfig struct {
	// 服务发现模式：static（静态地址），etcd（etcd 服务发现）
	Discovery string `json:"discovery" yaml:"discovery" toml:"discovery"`
	// 静态服务地址映射（discovery=static 时使用）
	// 格式：服务名 -> 地址（如 "user-service": "127.0.0.1:9001"）
	StaticAddresses map[string]string `json:"staticAddresses" yaml:"staticAddresses" toml:"staticAddresses"`
	// 连接超时时间 示例：10s
	Timeout string `json:"timeout" yaml:"timeout" toml:"timeout"`
	// 是否使用非安全连接（不加密）
	Insecure bool `json:"insecure" yaml:"insecure" toml:"insecure"`
	// 心跳时间 示例：10s
	KeepAliveTime string `json:"keepAliveTime" yaml:"keepAliveTime" toml:"keepAliveTime"`
	// 心跳超时时间 示例：3s
	KeepAliveTimeout string `json:"keepAliveTimeout" yaml:"keepAliveTimeout" toml:"keepAliveTimeout"`
	// 是否允许在没有活跃流时发送心跳
	PermitWithoutStream bool `json:"permitWithoutStream" yaml:"permitWithoutStream" toml:"permitWithoutStream"`
	// 负载均衡策略：round_robin, pick_first, weighted_round_robin
	LoadBalancing string `json:"loadBalancing" yaml:"loadBalancing" toml:"loadBalancing"`
	// 连接池大小（每个服务的连接数，默认为 1，建议设置为 2-4 以避免 HTTP/2 HPACK 并发问题）
	PoolSize int `json:"poolSize" yaml:"poolSize" toml:"poolSize"`
	// 健康检查间隔 示例：30s（默认 30s，设置为空或 0 则禁用）
	HealthCheckInterval string `json:"healthCheckInterval" yaml:"healthCheckInterval" toml:"healthCheckInterval"`
	// 连接失败后重试间隔 示例：5s（默认 5s）
	ReconnectInterval string `json:"reconnectInterval" yaml:"reconnectInterval" toml:"reconnectInterval"`
	// Etcd 配置（使用 etcd 服务发现时必需，全局共享）
	Etcd *EtcdConfig `json:"etcd" yaml:"etcd" toml:"etcd"`
}

// GrpcClientManager gRPC 客户端管理器
// 用于管理多个 gRPC 服务客户端，适合网关场景
type GrpcClientManager struct {
	clientPools         map[string]*clientPool // 服务名称 -> 连接池
	services            map[string]string      // 服务名称 -> 服务名称（用于记录已注册的服务）
	globalConfig        *GrpcClientConfig      // 全局配置（所有服务共享）
	etcdResolver        *grpc.EtcdResolver     // 共享的 etcd resolver
	mu                  sync.RWMutex
	healthCheckInterval time.Duration // 健康检查间隔
	reconnectInterval   time.Duration // 重连间隔
	healthCheckCtx      context.Context
	healthCheckCancel   context.CancelFunc
	healthCheckRunning  bool
}

// clientPool 连接池
type clientPool struct {
	serviceName  string         // 服务名称
	clients      []*grpc.Client // 连接池中的客户端
	index        uint64         // 轮询索引（使用原子操作）
	mu           sync.RWMutex
	unhealthy    []int        // 不健康的连接索引
	reconnecting map[int]bool // 正在重连的连接索引
}

// NewGrpcClientManager 创建 gRPC 客户端管理器
// config: 全局客户端配置（所有服务共享此配置）
func NewGrpcClientManager(config *GrpcClientConfig) (*GrpcClientManager, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}
	config = cloneGrpcClientConfig(config)

	// 设置默认连接池大小
	if config.PoolSize <= 0 {
		config.PoolSize = 1
	}

	// 解析健康检查间隔（默认 30s）
	var healthCheckInterval time.Duration
	if config.HealthCheckInterval != "" {
		var err error
		healthCheckInterval, err = time.ParseDuration(config.HealthCheckInterval)
		if err != nil {
			return nil, fmt.Errorf("failed to parse health check interval: %w", err)
		}
	} else {
		healthCheckInterval = 30 * time.Second // 默认 30 秒
	}

	// 解析重连间隔（默认 5s）
	var reconnectInterval time.Duration
	if config.ReconnectInterval != "" {
		var err error
		reconnectInterval, err = time.ParseDuration(config.ReconnectInterval)
		if err != nil {
			return nil, fmt.Errorf("failed to parse reconnect interval: %w", err)
		}
	} else {
		reconnectInterval = 5 * time.Second // 默认 5 秒
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &GrpcClientManager{
		clientPools:         make(map[string]*clientPool),
		services:            make(map[string]string),
		globalConfig:        config,
		healthCheckInterval: healthCheckInterval,
		reconnectInterval:   reconnectInterval,
		healthCheckCtx:      ctx,
		healthCheckCancel:   cancel,
	}

	// 如果配置了 etcd，创建共享的 resolver
	if config.Etcd != nil {
		dialTimeout, err := parseDurationOrDefault(config.Etcd.DialTimeout, defaultEtcdDialTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse etcd dial timeout: %w", err)
		}

		etcdConfig := grpc.EtcdConfig{
			Endpoints:   config.Etcd.Endpoints,
			DialTimeout: dialTimeout,
			Prefix:      config.Etcd.Prefix,
			Username:    config.Etcd.Username,
			Password:    config.Etcd.Password,
		}

		resolver, err := grpc.NewEtcdResolver(etcdConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create etcd resolver: %w", err)
		}

		// 注册 etcd resolver；同一 scheme 只能使用同一份配置。
		registeredResolver, err := grpc.RegisterResolverAndGet(grpc.EtcdScheme, resolver)
		if err != nil {
			resolver.Close()
			return nil, err
		}
		if registeredResolver != resolver {
			resolver.Close()
		}
		manager.etcdResolver = registeredResolver.(*grpc.EtcdResolver)
	}

	return manager, nil
}

// RegisterService 注册服务（只需要服务名称，配置使用全局配置）
// serviceName: 服务名称（使用服务发现时）或服务地址（直接连接时）
func (m *GrpcClientManager) RegisterService(serviceName string) error {
	if serviceName == "" {
		return errors.New("serviceName is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.services[serviceName] = serviceName
	logger.Info(context.Background(), "Registered gRPC service: service=%s", serviceName)
	return nil
}

// GetClient 获取客户端连接（从连接池中轮询获取）
// serviceName: 服务名称
func (m *GrpcClientManager) GetClient(ctx context.Context, serviceName string) (*grpc.Client, error) {
	m.mu.RLock()
	pool, exists := m.clientPools[serviceName]
	m.mu.RUnlock()

	if exists && pool != nil {
		client := pool.getClient()
		if client != nil && client.IsConnected() {
			return client, nil
		}
	}

	// 需要创建新连接池
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if pool, exists := m.clientPools[serviceName]; exists && pool != nil {
		client := pool.getClient()
		if client != nil && client.IsConnected() {
			return client, nil
		}
	}

	// 检查服务是否已注册
	if _, exists := m.services[serviceName]; !exists {
		return nil, fmt.Errorf("service not registered: %s", serviceName)
	}

	// 创建连接池
	pool, err := m.createClientPool(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to create client pool for service %s: %w", serviceName, err)
	}

	// 保存连接池
	m.clientPools[serviceName] = pool
	logger.Info(ctx, "Created gRPC client pool: service=%s, poolSize=%d", serviceName, m.globalConfig.PoolSize)

	client := pool.getClient()
	if client == nil {
		return nil, fmt.Errorf("no usable grpc client available for service %s", serviceName)
	}

	return client, nil
}

// GetConn 获取服务连接（便捷方法）
// serviceName: 服务名称
func (m *GrpcClientManager) GetConn(ctx context.Context, serviceName string) (*rpc.ClientConn, error) {
	client, err := m.GetClient(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("grpc client is nil for service %s", serviceName)
	}
	return client.GetConn(), nil
}

// createClient 创建客户端（内部方法）
func (m *GrpcClientManager) createClient(serviceName string) (*grpc.Client, error) {
	config := m.globalConfig

	// 解析超时时间
	var (
		timeout time.Duration
		err     error
	)
	if config.Timeout != "" {
		timeout, err = time.ParseDuration(config.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timeout: %w", err)
		}
	}

	// 解析 KeepAlive 时间
	var keepAliveTime time.Duration
	if config.KeepAliveTime != "" {
		keepAliveTime, err = time.ParseDuration(config.KeepAliveTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse keepAliveTime: %w", err)
		}
	}

	// 解析 KeepAlive 超时时间
	var keepAliveTimeout time.Duration
	if config.KeepAliveTimeout != "" {
		keepAliveTimeout, err = time.ParseDuration(config.KeepAliveTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse keepAliveTimeout: %w", err)
		}
	}

	// 确定连接地址
	// 如果是静态模式，从 StaticAddresses 中获取地址
	address := serviceName
	if config.Discovery == "static" && config.StaticAddresses != nil {
		if staticAddr, ok := config.StaticAddresses[serviceName]; ok {
			address = staticAddr
			logger.Info(context.Background(), "Using static address for service: service=%s, address=%s", serviceName, address)
		}
	}

	// 构建客户端配置
	clientConfig := grpc.ClientConfig{
		Address:  address, // 使用解析后的地址
		Timeout:  timeout,
		Insecure: config.Insecure,
	}

	// 设置 KeepAlive 配置
	if keepAliveTime > 0 || keepAliveTimeout > 0 {
		clientConfig.KeepAlive = &grpc.KeepAliveConfig{
			Time:                keepAliveTime,
			Timeout:             keepAliveTimeout,
			PermitWithoutStream: config.PermitWithoutStream,
		}
	}

	// 设置负载均衡策略
	if config.LoadBalancing != "" {
		clientConfig.LoadBalancing = grpc.LoadBalancingPolicy(config.LoadBalancing)
	} else {
		// 如果使用服务发现，默认使用轮询策略
		if config.Etcd != nil {
			clientConfig.LoadBalancing = grpc.PolicyRoundRobin
		}
	}

	// 如果配置了 etcd，使用共享的 resolver
	if config.Etcd != nil && m.etcdResolver != nil {
		clientConfig.ServiceDiscovery = m.etcdResolver
	}

	// 创建客户端
	client, err := grpc.NewClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc client: %w", err)
	}

	return client, nil
}

// createClientPool 创建连接池（内部方法）
func (m *GrpcClientManager) createClientPool(ctx context.Context, serviceName string) (*clientPool, error) {
	poolSize := m.globalConfig.PoolSize
	if poolSize <= 0 {
		poolSize = 1
	}

	pool := &clientPool{
		serviceName:  serviceName,
		clients:      make([]*grpc.Client, 0, poolSize),
		unhealthy:    make([]int, 0),
		reconnecting: make(map[int]bool),
	}

	for i := 0; i < poolSize; i++ {
		client, err := m.createClient(serviceName)
		if err != nil {
			// 关闭已创建的连接
			for _, c := range pool.clients {
				c.Close()
			}
			return nil, fmt.Errorf("failed to create client %d: %w", i, err)
		}

		if err := client.Connect(ctx); err != nil {
			// 关闭已创建的连接
			for _, c := range pool.clients {
				c.Close()
			}
			return nil, fmt.Errorf("failed to connect client %d: %w", i, err)
		}

		pool.clients = append(pool.clients, client)
	}

	return pool, nil
}

// getClient 从连接池中获取客户端（轮询方式）
func (p *clientPool) getClient() *grpc.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.clients) == 0 {
		return nil
	}

	start := atomic.AddUint64(&p.index, 1) - 1
	for offset := 0; offset < len(p.clients); offset++ {
		idx := int((start + uint64(offset)) % uint64(len(p.clients)))
		client := p.clients[idx]
		if client != nil && client.IsConnected() {
			return client
		}
	}
	return nil
}

// close 关闭连接池中的所有连接
func (p *clientPool) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for _, client := range p.clients {
		if client == nil {
			continue
		}
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	p.clients = nil
	p.unhealthy = nil
	p.reconnecting = make(map[int]bool)

	if len(errs) > 0 {
		return fmt.Errorf("failed to close some clients: %w", errors.Join(errs...))
	}
	return nil
}

func (p *clientPool) finishReconnect(idx int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishReconnectLocked(idx)
}

func (p *clientPool) finishReconnectLocked(idx int) {
	if p.reconnecting != nil {
		delete(p.reconnecting, idx)
	}
}

// ConnectAll 连接所有已注册的客户端
func (m *GrpcClientManager) ConnectAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for serviceName := range m.services {
		if _, exists := m.clientPools[serviceName]; exists {
			continue // 已经连接
		}

		pool, err := m.createClientPool(ctx, serviceName)
		if err != nil {
			errs = append(errs, fmt.Errorf("service %s: %w", serviceName, err))
			continue
		}

		m.clientPools[serviceName] = pool
		logger.Info(ctx, "Connected gRPC client pool: service=%s, poolSize=%d", serviceName, m.globalConfig.PoolSize)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to connect some clients: %w", errors.Join(errs...))
	}

	return nil
}

// CloseClient 关闭指定服务的客户端
func (m *GrpcClientManager) CloseClient(serviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, exists := m.clientPools[serviceName]
	if !exists {
		return nil // 不存在，无需关闭
	}

	if err := pool.close(); err != nil {
		logger.Error(context.Background(), "Failed to close client pool: service=%s, error=%v", serviceName, err)
		return err
	}

	delete(m.clientPools, serviceName)
	logger.Info(context.Background(), "Closed gRPC client pool: service=%s", serviceName)
	return nil
}

// CloseAll 关闭所有客户端
func (m *GrpcClientManager) CloseAll() error {
	// 先停止健康检查
	m.StopHealthCheck()

	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for serviceName, pool := range m.clientPools {
		if err := pool.close(); err != nil {
			errs = append(errs, fmt.Errorf("service %s: %w", serviceName, err))
		} else {
			logger.Info(context.Background(), "Closed gRPC client pool: service=%s", serviceName)
		}
	}

	// 清空
	m.clientPools = make(map[string]*clientPool)
	// etcdResolver is registered in gRPC's process-global resolver registry.
	// Closing it here would leave the global builder pointing at a closed client.
	m.etcdResolver = nil

	if len(errs) > 0 {
		return fmt.Errorf("failed to close some clients: %w", errors.Join(errs...))
	}

	return nil
}

// HealthCheck 健康检查指定服务
func (m *GrpcClientManager) HealthCheck(ctx context.Context, serviceName, service string) error {
	client, err := m.GetClient(ctx, serviceName)
	if err != nil {
		return err
	}

	_, err = client.HealthCheck(ctx, service)
	return err
}

// ListServices 列出所有已注册的服务名称
func (m *GrpcClientManager) ListServices() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	services := make([]string, 0, len(m.services))
	for serviceName := range m.services {
		services = append(services, serviceName)
	}
	return services
}

// IsConnected 检查指定服务是否已连接
func (m *GrpcClientManager) IsConnected(serviceName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pool, exists := m.clientPools[serviceName]
	if !exists || pool == nil || len(pool.clients) == 0 {
		return false
	}

	// 检查连接池中是否有可用连接
	for _, client := range pool.clients {
		if client != nil && client.IsConnected() {
			return true
		}
	}
	return false
}

// StartHealthCheck 启动后台健康检查
// 定期检查所有连接池中的连接状态，自动重连不健康的连接
func (m *GrpcClientManager) StartHealthCheck() {
	if m.healthCheckInterval <= 0 {
		logger.Info(context.Background(), "Health check disabled (interval <= 0)")
		return
	}

	m.mu.Lock()
	if m.healthCheckRunning {
		m.mu.Unlock()
		return
	}
	m.healthCheckRunning = true
	m.healthCheckCtx, m.healthCheckCancel = context.WithCancel(context.Background())
	m.mu.Unlock()

	logger.Info(context.Background(), "Starting gRPC client health check: interval=%v", m.healthCheckInterval)

	go m.healthCheckLoop()
}

// StopHealthCheck 停止后台健康检查
func (m *GrpcClientManager) StopHealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.healthCheckRunning {
		return
	}

	if m.healthCheckCancel != nil {
		m.healthCheckCancel()
	}
	m.healthCheckRunning = false
	logger.Info(context.Background(), "Stopped gRPC client health check")
}

// healthCheckLoop 健康检查循环
func (m *GrpcClientManager) healthCheckLoop() {
	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.healthCheckCtx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck 执行一次健康检查
func (m *GrpcClientManager) performHealthCheck() {
	m.mu.RLock()
	pools := make(map[string]*clientPool, len(m.clientPools))
	for name, pool := range m.clientPools {
		pools[name] = pool
	}
	m.mu.RUnlock()

	for serviceName, pool := range pools {
		m.checkPoolHealth(serviceName, pool)
	}
}

// checkPoolHealth 检查单个连接池的健康状态
func (m *GrpcClientManager) checkPoolHealth(serviceName string, pool *clientPool) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	unhealthyIndices := make([]int, 0)

	for i, client := range pool.clients {
		if client == nil {
			unhealthyIndices = append(unhealthyIndices, i)
			continue
		}

		// 检查连接状态
		if !client.IsConnected() {
			logger.Warn(context.Background(), "Unhealthy connection detected: service=%s, index=%d", serviceName, i)
			unhealthyIndices = append(unhealthyIndices, i)
		}
	}

	pool.unhealthy = unhealthyIndices

	reconnectIndices := make([]int, 0, len(unhealthyIndices))
	for _, idx := range unhealthyIndices {
		if pool.reconnecting == nil {
			pool.reconnecting = make(map[int]bool)
		}
		if pool.reconnecting[idx] {
			continue
		}
		pool.reconnecting[idx] = true
		reconnectIndices = append(reconnectIndices, idx)
	}

	// 如果有不健康的连接，尝试重连
	if len(reconnectIndices) > 0 {
		go m.reconnectUnhealthyClients(serviceName, pool, reconnectIndices)
	}
}

// reconnectUnhealthyClients 重连不健康的客户端
func (m *GrpcClientManager) reconnectUnhealthyClients(serviceName string, pool *clientPool, indices []int) {
	for _, idx := range indices {
		select {
		case <-m.healthCheckCtx.Done():
			return
		default:
		}

		pool.mu.Lock()
		if idx >= len(pool.clients) {
			pool.finishReconnectLocked(idx)
			pool.mu.Unlock()
			continue
		}

		oldClient := pool.clients[idx]
		pool.mu.Unlock()

		// 关闭旧连接
		if oldClient != nil {
			oldClient.Close()
		}

		// 创建新客户端
		newClient, err := m.createClient(serviceName)
		if err != nil {
			logger.Error(context.Background(), "Failed to create new client for reconnection: service=%s, index=%d, error=%v", serviceName, idx, err)
			pool.finishReconnect(idx)
			time.Sleep(m.reconnectInterval)
			continue
		}

		// 连接
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := newClient.Connect(ctx); err != nil {
			cancel()
			logger.Error(context.Background(), "Failed to reconnect client: service=%s, index=%d, error=%v", serviceName, idx, err)
			newClient.Close()
			pool.finishReconnect(idx)
			time.Sleep(m.reconnectInterval)
			continue
		}
		cancel()

		// 替换客户端
		pool.mu.Lock()
		if idx < len(pool.clients) {
			pool.clients[idx] = newClient
			// 从不健康列表中移除
			for i, unhealthyIdx := range pool.unhealthy {
				if unhealthyIdx == idx {
					pool.unhealthy = append(pool.unhealthy[:i], pool.unhealthy[i+1:]...)
					break
				}
			}
			pool.finishReconnectLocked(idx)
		} else {
			newClient.Close()
			pool.finishReconnectLocked(idx)
		}
		pool.mu.Unlock()

		logger.Info(context.Background(), "Reconnected client successfully: service=%s, index=%d", serviceName, idx)
	}
}

// GetPoolStatus 获取连接池状态信息
func (m *GrpcClientManager) GetPoolStatus() map[string]PoolStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]PoolStatus)
	for name, pool := range m.clientPools {
		pool.mu.RLock()
		healthy := 0
		unhealthy := 0
		for _, client := range pool.clients {
			if client != nil && client.IsConnected() {
				healthy++
			} else {
				unhealthy++
			}
		}
		status[name] = PoolStatus{
			ServiceName: name,
			Total:       len(pool.clients),
			Healthy:     healthy,
			Unhealthy:   unhealthy,
		}
		pool.mu.RUnlock()
	}
	return status
}

// PoolStatus 连接池状态
type PoolStatus struct {
	ServiceName string `json:"serviceName"`
	Total       int    `json:"total"`
	Healthy     int    `json:"healthy"`
	Unhealthy   int    `json:"unhealthy"`
}

// ==================== 单客户端封装（向后兼容） ====================

// GrpcClient 单个 gRPC 客户端（向后兼容，推荐使用 GrpcClientManager）
type GrpcClient struct {
	client           *grpc.Client
	config           *GrpcClientConfig
	etcdResolver     *grpc.EtcdResolver
	serviceDiscovery grpc.ServiceDiscovery
}

// NewGrpcClient 创建新的 gRPC 客户端（单客户端，向后兼容）
// serviceName: 服务名称（使用服务发现时）或服务地址（直接连接时）
// config: 客户端配置
// 推荐在网关场景使用 NewGrpcClientManager
func NewGrpcClient(serviceName string, config *GrpcClientConfig) (*GrpcClient, error) {
	if serviceName == "" {
		return nil, errors.New("serviceName is required")
	}
	if config == nil {
		return nil, errors.New("config is nil")
	}
	config = cloneGrpcClientConfig(config)

	// 解析超时时间
	var timeout time.Duration
	var err error
	if config.Timeout != "" {
		timeout, err = time.ParseDuration(config.Timeout)
		if err != nil {
			logger.Error(context.Background(), "Failed to parse GrpcClientConfig.Timeout: %v", err)
			return nil, err
		}
	}

	// 解析 KeepAlive 时间
	var keepAliveTime time.Duration
	if config.KeepAliveTime != "" {
		keepAliveTime, err = time.ParseDuration(config.KeepAliveTime)
		if err != nil {
			logger.Error(context.Background(), "Failed to parse GrpcClientConfig.KeepAliveTime: %v", err)
			return nil, err
		}
	}

	// 解析 KeepAlive 超时时间
	var keepAliveTimeout time.Duration
	if config.KeepAliveTimeout != "" {
		keepAliveTimeout, err = time.ParseDuration(config.KeepAliveTimeout)
		if err != nil {
			logger.Error(context.Background(), "Failed to parse GrpcClientConfig.KeepAliveTimeout: %v", err)
			return nil, err
		}
	}

	// 构建客户端配置
	clientConfig := grpc.ClientConfig{
		Address:  serviceName, // 使用传入的服务名称
		Timeout:  timeout,
		Insecure: config.Insecure,
	}

	// 设置 KeepAlive 配置
	if keepAliveTime > 0 || keepAliveTimeout > 0 {
		clientConfig.KeepAlive = &grpc.KeepAliveConfig{
			Time:                keepAliveTime,
			Timeout:             keepAliveTimeout,
			PermitWithoutStream: config.PermitWithoutStream,
		}
	}

	// 设置负载均衡策略
	if config.LoadBalancing != "" {
		clientConfig.LoadBalancing = grpc.LoadBalancingPolicy(config.LoadBalancing)
	} else {
		// 如果使用服务发现，默认使用轮询策略
		if config.Etcd != nil {
			clientConfig.LoadBalancing = grpc.PolicyRoundRobin
		}
	}

	// 如果配置了 etcd，使用 etcd 服务发现
	var etcdResolver *grpc.EtcdResolver
	if config.Etcd != nil {
		dialTimeout, err := parseDurationOrDefault(config.Etcd.DialTimeout, defaultEtcdDialTimeout)
		if err != nil {
			logger.Error(context.Background(), "Failed to parse GrpcClientConfig.Etcd.DialTimeout: %v", err)
			return nil, err
		}

		// 创建 etcd resolver 配置
		etcdConfig := grpc.EtcdConfig{
			Endpoints:   config.Etcd.Endpoints,
			DialTimeout: dialTimeout,
			Prefix:      config.Etcd.Prefix,
			Username:    config.Etcd.Username,
			Password:    config.Etcd.Password,
		}

		// 创建 etcd resolver
		etcdResolver, err = grpc.NewEtcdResolver(etcdConfig)
		if err != nil {
			logger.Error(context.Background(), "Failed to create etcd resolver: %v", err)
			return nil, err
		}

		// 注册 etcd resolver；同一 scheme 只能使用同一份配置。
		registeredResolver, err := grpc.RegisterResolverAndGet(grpc.EtcdScheme, etcdResolver)
		if err != nil {
			etcdResolver.Close()
			return nil, err
		}
		if registeredResolver != etcdResolver {
			etcdResolver.Close()
		}
		etcdResolver = registeredResolver.(*grpc.EtcdResolver)

		// 设置服务发现
		clientConfig.ServiceDiscovery = etcdResolver
	}

	// 创建客户端
	client, err := grpc.NewClient(clientConfig)
	if err != nil {
		logger.Error(context.Background(), "Failed to create grpc client: %v", err)
		return nil, err
	}

	return &GrpcClient{
		client:           client,
		config:           config,
		etcdResolver:     etcdResolver,
		serviceDiscovery: etcdResolver,
	}, nil
}

func cloneGrpcClientConfig(config *GrpcClientConfig) *GrpcClientConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.StaticAddresses != nil {
		cloned.StaticAddresses = make(map[string]string, len(config.StaticAddresses))
		for service, address := range config.StaticAddresses {
			cloned.StaticAddresses[service] = address
		}
	}
	if config.Etcd != nil {
		etcd := *config.Etcd
		etcd.Endpoints = append([]string(nil), config.Etcd.Endpoints...)
		cloned.Etcd = &etcd
	}
	return &cloned
}

// Connect 连接到 gRPC 服务器
func (c *GrpcClient) Connect(ctx context.Context) error {
	if c.client == nil {
		return errors.New("client is nil")
	}

	logger.Info(ctx, "Connecting to gRPC service")
	if err := c.client.Connect(ctx); err != nil {
		logger.Error(ctx, "Failed to connect to gRPC service: %v", err)
		return err
	}

	logger.Info(ctx, "Connected to gRPC service successfully")
	return nil
}

// GetConn 获取 gRPC 连接（用于创建服务客户端）
func (c *GrpcClient) GetConn() *rpc.ClientConn {
	if c.client == nil {
		return nil
	}
	return c.client.GetConn()
}

// IsConnected 检查是否已连接
func (c *GrpcClient) IsConnected() bool {
	if c.client == nil {
		return false
	}
	return c.client.IsConnected()
}

// HealthCheck 健康检查
func (c *GrpcClient) HealthCheck(ctx context.Context, service string) error {
	if c.client == nil {
		return errors.New("client is nil")
	}
	_, err := c.client.HealthCheck(ctx, service)
	return err
}

// Close 关闭客户端连接
func (c *GrpcClient) Close() error {
	var errs []error
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			logger.Error(context.Background(), "Failed to close grpc client: %v", err)
			errs = append(errs, err)
		}
		c.client = nil
	}
	// etcdResolver is registered in gRPC's process-global resolver registry.
	// Closing it here would leave the global builder pointing at a closed client.
	c.etcdResolver = nil
	c.serviceDiscovery = nil

	logger.Info(context.Background(), "gRPC client closed")
	if len(errs) > 0 {
		return fmt.Errorf("failed to close grpc client: %w", errors.Join(errs...))
	}
	return nil
}

// WithTimeout 创建带超时的 context
func (c *GrpcClient) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if c.client == nil {
		return context.WithTimeout(ctx, timeout)
	}
	return c.client.WithTimeout(ctx, timeout)
}

// WithDeadline 创建带截止时间的 context
func (c *GrpcClient) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if c.client == nil {
		return context.WithDeadline(ctx, deadline)
	}
	return c.client.WithDeadline(ctx, deadline)
}
