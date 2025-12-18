package redis

import (
	"context"
	"fmt"
	"time"

	"quickgo/logger"

	redisClient "github.com/redis/go-redis/v9"
)

// Client Redis 客户端封装
type Client struct {
	name   string
	client *redisClient.Client
	config *RedisConfig
}

// NewClient 创建 Redis 客户端
func NewClient(config *RedisConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config is nil")
	}

	if config.Name == "" {
		return nil, fmt.Errorf("database name is required")
	}

	ctx := context.Background()
	logger.Info(ctx, "Initializing Redis client: name=%s", config.Name)

	// 构建连接地址
	addr := config.Addr
	if addr == "" {
		host := config.Host
		if host == "" {
			host = "localhost"
		}
		port := config.Port
		if port == 0 {
			port = 6379
		}
		addr = fmt.Sprintf("%s:%d", host, port)
	}

	// 配置选项
	options := &redisClient.Options{
		Addr:     addr,
		Password: config.Password,
		DB:       config.DB,
		Username: config.Username,
	}

	// 连接池配置
	if config.PoolSize > 0 {
		options.PoolSize = config.PoolSize
	} else {
		options.PoolSize = 10 // 默认值
	}

	if config.MinIdleConns > 0 {
		options.MinIdleConns = config.MinIdleConns
	}

	// 解析并设置连接最大生存时间
	if config.MaxConnAge != "" {
		maxConnAge, err := time.ParseDuration(config.MaxConnAge)
		if err != nil {
			return nil, fmt.Errorf("failed to parse MaxConnAge %s: %w", config.MaxConnAge, err)
		}
		if maxConnAge > 0 {
			options.ConnMaxLifetime = maxConnAge
		}
	}

	// 解析并设置连接池超时时间
	if config.PoolTimeout != "" {
		poolTimeout, err := time.ParseDuration(config.PoolTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PoolTimeout %s: %w", config.PoolTimeout, err)
		}
		if poolTimeout > 0 {
			options.PoolTimeout = poolTimeout
		}
	} else {
		options.PoolTimeout = 4 * time.Second // 默认值
	}

	// 解析并设置空闲连接超时时间
	if config.IdleTimeout != "" {
		idleTimeout, err := time.ParseDuration(config.IdleTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse IdleTimeout %s: %w", config.IdleTimeout, err)
		}
		if idleTimeout > 0 {
			options.ConnMaxIdleTime = idleTimeout
		}
	}

	// 解析并设置连接超时时间
	if config.DialTimeout != "" {
		dialTimeout, err := time.ParseDuration(config.DialTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DialTimeout %s: %w", config.DialTimeout, err)
		}
		if dialTimeout > 0 {
			options.DialTimeout = dialTimeout
		}
	} else {
		options.DialTimeout = 5 * time.Second // 默认值
	}

	// 解析并设置读取超时时间
	if config.ReadTimeout != "" {
		readTimeout, err := time.ParseDuration(config.ReadTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ReadTimeout %s: %w", config.ReadTimeout, err)
		}
		if readTimeout > 0 {
			options.ReadTimeout = readTimeout
		}
	} else {
		options.ReadTimeout = 3 * time.Second // 默认值
	}

	// 解析并设置写入超时时间
	if config.WriteTimeout != "" {
		writeTimeout, err := time.ParseDuration(config.WriteTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse WriteTimeout %s: %w", config.WriteTimeout, err)
		}
		if writeTimeout > 0 {
			options.WriteTimeout = writeTimeout
		}
	} else {
		options.WriteTimeout = 3 * time.Second // 默认值
	}

	// TLS 配置（如果需要，可以在这里添加 TLS 配置）
	// if config.TLS {
	//     options.TLSConfig = &tls.Config{}
	// }

	// 创建客户端
	client := redisClient.NewClient(options)

	// 测试连接（使用带超时的 context，确保不会无限等待）
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		// 连接失败，关闭已创建的客户端
		client.Close()
		return nil, fmt.Errorf("failed to ping Redis (connection test failed): %w", err)
	}

	logger.Info(ctx, "Redis client initialized successfully: name=%s, addr=%s, db=%d", config.Name, addr, config.DB)

	return &Client{
		name:   config.Name,
		client: client,
		config: config,
	}, nil
}

// GetClient 获取 Redis 客户端
func (c *Client) GetClient() *redisClient.Client {
	return c.client
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

	ctx := context.Background()
	logger.Info(ctx, "Closing Redis client: name=%s", c.name)

	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close Redis client: %w", err)
	}

	logger.Info(ctx, "Redis client closed: name=%s", c.name)
	return nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}
