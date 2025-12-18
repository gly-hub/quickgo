package mongodb

import (
	"context"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/mongo"
	"quickgo/logger"
)

// Manager MongoDB 多客户端管理器
type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager 创建 MongoDB 管理器
func NewManager(config *MongoManagerConfig) (*Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("mongodb manager config is nil")
	}

	manager := &Manager{
		clients: make(map[string]*Client),
	}

	ctx := context.Background()
	logger.Info(ctx, "Initializing MongoDB Manager: database_count=%d", len(config.Databases))

	// 初始化所有数据库客户端
	// 注意：如果任何一个 MongoDB 连接失败，整个 Manager 创建失败，服务无法启动
	for i := range config.Databases {
		dbConfig := &config.Databases[i]
		if dbConfig.Name == "" {
			return nil, fmt.Errorf("database[%d] name is required", i)
		}

		logger.Info(ctx, "Connecting to MongoDB: name=%s", dbConfig.Name)

		client, err := NewClient(dbConfig)
		if err != nil {
			// 连接失败，返回错误，阻止服务启动
			return nil, fmt.Errorf("failed to connect to MongoDB %s (service cannot start without MongoDB): %w", dbConfig.Name, err)
		}

		manager.clients[dbConfig.Name] = client
		logger.Info(ctx, "MongoDB client connected successfully: name=%s", dbConfig.Name)
	}

	if len(manager.clients) == 0 {
		return nil, fmt.Errorf("no MongoDB databases configured or all MongoDB connections failed")
	}

	logger.Info(ctx, "MongoDB Manager initialized successfully: total_clients=%d", len(manager.clients))

	return manager, nil
}

// GetClient 获取指定名称的数据库客户端
func (m *Manager) GetClient(name string) (*Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, exists := m.clients[name]
	if !exists {
		return nil, fmt.Errorf("mongodb client not found: name=%s", name)
	}

	return client, nil
}

// GetDB 获取指定名称的 MongoDB 数据库实例（便捷方法）
func (m *Manager) GetDB(name string) (*mongo.Database, error) {
	client, err := m.GetClient(name)
	if err != nil {
		return nil, err
	}
	return client.GetDB(), nil
}

// RegisterClient 注册新的数据库客户端（动态添加）
func (m *Manager) RegisterClient(config *MongoConfig) error {
	if config == nil {
		return fmt.Errorf("mongodb config is nil")
	}

	if config.Name == "" {
		return fmt.Errorf("database name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if _, exists := m.clients[config.Name]; exists {
		return fmt.Errorf("mongodb client already exists: name=%s", config.Name)
	}

	ctx := context.Background()
	logger.Info(ctx, "Registering new MongoDB client: name=%s", config.Name)

	client, err := NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	m.clients[config.Name] = client
	logger.Info(ctx, "MongoDB client registered successfully: name=%s", config.Name)

	return nil
}

// ListClients 列出所有已注册的客户端名称
func (m *Manager) ListClients() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}

	return names
}

// HealthCheck 健康检查（检查所有客户端）
func (m *Manager) HealthCheck(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, client := range m.clients {
		if err := client.HealthCheck(ctx); err != nil {
			errors = append(errors, fmt.Errorf("database %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("health check failed: %v", errors)
	}

	return nil
}

// Close 关闭所有数据库连接
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.Background()
	logger.Info(ctx, "Closing MongoDB Manager: total_clients=%d", len(m.clients))

	var errors []error
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close client %s: %w", name, err))
			logger.Error(ctx, "Failed to close MongoDB client: name=%s, error=%v", name, err)
		} else {
			logger.Info(ctx, "MongoDB client closed: name=%s", name)
		}
	}

	m.clients = make(map[string]*Client)

	if len(errors) > 0 {
		return fmt.Errorf("failed to close some clients: %v", errors)
	}

	logger.Info(ctx, "MongoDB Manager closed successfully")
	return nil
}
