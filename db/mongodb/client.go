package mongodb

import (
	"context"
	"fmt"
	"time"

	"gly-hub/go-dandelion/quickgo/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Client MongoDB 客户端封装
type Client struct {
	name   string
	client *mongo.Client
	db     *mongo.Database
	config *MongoConfig
}

// NewClient 创建 MongoDB 客户端
func NewClient(config *MongoConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("mongodb config is nil")
	}

	if config.Name == "" {
		return nil, fmt.Errorf("database name is required")
	}

	ctx := context.Background()
	logger.Info(ctx, "Initializing MongoDB client: name=%s", config.Name)

	// 构建连接 URI
	uri := config.URI
	if uri == "" {
		var err error
		uri, err = buildURI(config)
		if err != nil {
			return nil, fmt.Errorf("failed to build URI: %w", err)
		}
	}

	// 配置客户端选项
	clientOptions := options.Client().ApplyURI(uri)

	// 连接池配置
	if config.MaxPoolSize > 0 {
		clientOptions.SetMaxPoolSize(config.MaxPoolSize)
	}
	if config.MinPoolSize > 0 {
		clientOptions.SetMinPoolSize(config.MinPoolSize)
	}
	
	// 解析并设置连接最大空闲时间
	if config.MaxConnIdleTime != "" {
		maxConnIdleTime, err := time.ParseDuration(config.MaxConnIdleTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse MaxConnIdleTime %s: %w", config.MaxConnIdleTime, err)
		}
		if maxConnIdleTime > 0 {
			clientOptions.SetMaxConnIdleTime(maxConnIdleTime)
		}
	}
	
	// 解析并设置连接超时时间
	if config.ConnectTimeout != "" {
		connectTimeout, err := time.ParseDuration(config.ConnectTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ConnectTimeout %s: %w", config.ConnectTimeout, err)
		}
		if connectTimeout > 0 {
			clientOptions.SetConnectTimeout(connectTimeout)
		}
	}
	
	// 解析并设置 Socket 超时时间
	if config.SocketTimeout != "" {
		socketTimeout, err := time.ParseDuration(config.SocketTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SocketTimeout %s: %w", config.SocketTimeout, err)
		}
		if socketTimeout > 0 {
			clientOptions.SetSocketTimeout(socketTimeout)
		}
	}

	// 添加其他选项
	for k, v := range config.Options {
		clientOptions.SetAppName(k + "=" + v)
	}

	// 创建客户端
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// 测试连接（使用带超时的 context，确保不会无限等待）
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	
	if err := client.Ping(pingCtx, nil); err != nil {
		// 连接失败，关闭已创建的客户端
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping MongoDB (connection test failed): %w", err)
	}

	// 获取数据库实例
	dbName := config.Database
	if dbName == "" {
		dbName = "test"
	}
	db := client.Database(dbName)

	logger.Info(ctx, "MongoDB client initialized successfully: name=%s, database=%s", config.Name, dbName)

	return &Client{
		name:   config.Name,
		client: client,
		db:     db,
		config: config,
	}, nil
}

// GetClient 获取 MongoDB 客户端
func (c *Client) GetClient() *mongo.Client {
	return c.client
}

// GetDB 获取 MongoDB 数据库实例
func (c *Client) GetDB() *mongo.Database {
	return c.db
}

// GetName 获取数据库名称
func (c *Client) GetName() string {
	return c.name
}

// Close 关闭数据库连接
func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Info(ctx, "Closing MongoDB client: name=%s", c.name)

	if err := c.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect MongoDB: %w", err)
	}

	logger.Info(ctx, "MongoDB client closed: name=%s", c.name)
	return nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("mongodb client is nil")
	}

	if err := c.client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// buildURI 构建 MongoDB URI
func buildURI(config *MongoConfig) (string, error) {
	if config.Host == "" {
		return "", fmt.Errorf("host is required")
	}

	port := config.Port
	if port == 0 {
		port = 27017
	}

	uri := fmt.Sprintf("mongodb://")

	// 添加认证信息
	if config.Username != "" && config.Password != "" {
		uri += fmt.Sprintf("%s:%s@", config.Username, config.Password)
	}

	// 添加主机和端口
	uri += fmt.Sprintf("%s:%d", config.Host, port)

	// 添加数据库
	if config.Database != "" {
		uri += "/" + config.Database
	}

	// 添加认证源
	params := make(map[string]string)
	if config.AuthSource != "" {
		params["authSource"] = config.AuthSource
	}

	// 添加其他选项
	for k, v := range config.Options {
		params[k] = v
	}

	// 构建参数字符串
	if len(params) > 0 {
		paramStr := ""
		for k, v := range params {
			if paramStr != "" {
				paramStr += "&"
			}
			paramStr += fmt.Sprintf("%s=%s", k, v)
		}
		uri += "?" + paramStr
	}

	return uri, nil
}

