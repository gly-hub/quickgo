package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"quickgo/logger"
)

// ServiceRegistry 服务注册接口
type ServiceRegistry interface {
	// Register 注册服务
	Register(ctx context.Context, serviceName, address string, metadata map[string]string) error
	// Deregister 注销服务
	Deregister(ctx context.Context, serviceName, address string) error
	// KeepAlive 保持服务活跃（心跳）
	KeepAlive(ctx context.Context, serviceName, address string) error
	// Close 关闭注册中心连接
	Close() error
}

// ServiceInfo 服务信息
type ServiceInfo struct {
	Name     string
	Address  string
	Metadata map[string]string
	Weight   int // 权重，用于负载均衡
}

// StaticRegistry 静态服务注册（用于测试，实际不注册到注册中心）
type StaticRegistry struct {
	services map[string][]ServiceInfo
	mu       sync.RWMutex
}

// NewStaticRegistry 创建静态服务注册
func NewStaticRegistry() *StaticRegistry {
	return &StaticRegistry{
		services: make(map[string][]ServiceInfo),
	}
}

// Register 注册服务
func (r *StaticRegistry) Register(ctx context.Context, serviceName, address string, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := ServiceInfo{
		Name:     serviceName,
		Address:  address,
		Metadata: metadata,
		Weight:   1,
	}

	if weight, ok := metadata["weight"]; ok {
		if w, err := parseInt(weight); err == nil {
			info.Weight = w
		}
	}

	r.services[serviceName] = append(r.services[serviceName], info)
	logger.Info(ctx, "Service registered: service=%s, address=%s", serviceName, address)
	return nil
}

// Deregister 注销服务
func (r *StaticRegistry) Deregister(ctx context.Context, serviceName, address string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	services, ok := r.services[serviceName]
	if !ok {
		return nil
	}

	for i, s := range services {
		if s.Address == address {
			r.services[serviceName] = append(services[:i], services[i+1:]...)
			logger.Info(ctx, "Service deregistered: service=%s, address=%s", serviceName, address)
			return nil
		}
	}

	return nil
}

// KeepAlive 保持服务活跃
func (r *StaticRegistry) KeepAlive(ctx context.Context, serviceName, address string) error {
	// 静态注册不需要心跳
	return nil
}

// Close 关闭注册中心连接
func (r *StaticRegistry) Close() error {
	return nil
}

// GetServices 获取服务列表
func (r *StaticRegistry) GetServices(serviceName string) []ServiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services[serviceName]
}

// parseInt 解析整数（辅助函数）
func parseInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int32:
		return int(val), nil
	case int64:
		return int(val), nil
	case string:
		var result int
		_, err := fmt.Sscanf(val, "%d", &result)
		return result, err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// ServiceRegistrar 服务注册器（用于服务端自动注册）
type ServiceRegistrar struct {
	registry        ServiceRegistry
	serviceName     string
	address         string
	metadata        map[string]string
	keepAliveTicker *time.Ticker
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
}

// NewServiceRegistrar 创建服务注册器
func NewServiceRegistrar(registry ServiceRegistry, serviceName, address string, metadata map[string]string) *ServiceRegistrar {
	ctx, cancel := context.WithCancel(context.Background())
	return &ServiceRegistrar{
		registry:    registry,
		serviceName: serviceName,
		address:     address,
		metadata:    metadata,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Register 注册服务
func (sr *ServiceRegistrar) Register(ctx context.Context) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if err := sr.registry.Register(ctx, sr.serviceName, sr.address, sr.metadata); err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	logger.Info(ctx, "Service registered successfully: service=%s, address=%s", sr.serviceName, sr.address)
	return nil
}

// StartKeepAlive 启动心跳保持服务活跃
func (sr *ServiceRegistrar) StartKeepAlive(interval time.Duration) {
	if interval == 0 {
		interval = 30 * time.Second
	}

	sr.mu.Lock()
	sr.keepAliveTicker = time.NewTicker(interval)
	sr.mu.Unlock()

	go func() {
		for {
			select {
			case <-sr.ctx.Done():
				return
			case <-sr.keepAliveTicker.C:
				if err := sr.registry.KeepAlive(sr.ctx, sr.serviceName, sr.address); err != nil {
					logger.Error(sr.ctx, "KeepAlive failed: service=%s, address=%s", sr.serviceName, sr.address, err)
				}
			}
		}
	}()
}

// Deregister 注销服务
func (sr *ServiceRegistrar) Deregister(ctx context.Context) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// 停止心跳
	if sr.keepAliveTicker != nil {
		sr.keepAliveTicker.Stop()
	}

	if err := sr.registry.Deregister(ctx, sr.serviceName, sr.address); err != nil {
		return fmt.Errorf("failed to deregister service: %w", err)
	}

	logger.Info(ctx, "Service deregistered: service=%s, address=%s", sr.serviceName, sr.address)
	return nil
}

// Close 关闭注册器
func (sr *ServiceRegistrar) Close() error {
	sr.cancel()
	return sr.registry.Close()
}
