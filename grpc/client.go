package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	"gly-hub/go-dandelion/quickgo/logger"
	"gly-hub/go-dandelion/quickgo/tracing"
)

// Client gRPC客户端封装
type Client struct {
	conn    *grpc.ClientConn
	address string
	options []grpc.DialOption
	timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc
}

// ClientConfig 客户端配置
type ClientConfig struct {
	Address          string              // 服务器地址，格式：host:port 或 scheme://service-name（使用服务发现时）
	Timeout          time.Duration       // 连接超时时间
	Insecure         bool                // 是否使用非安全连接（不加密）
	TLS              *TLSConfig          // TLS配置（如果 Insecure=false）
	Options          []grpc.DialOption   // 自定义 DialOption
	KeepAlive        *KeepAliveConfig    // KeepAlive配置
	ServiceDiscovery ServiceDiscovery    // 服务发现（可选）
	LoadBalancing    LoadBalancingPolicy // 负载均衡策略
}

// TLSConfig TLS配置
type TLSConfig struct {
	CertFile   string // 证书文件路径
	KeyFile    string // 私钥文件路径
	CAFile     string // CA证书文件路径（可选，用于验证服务器证书）
	ServerName string // 服务器名称（用于验证服务器证书）
}

// KeepAliveConfig KeepAlive配置
type KeepAliveConfig struct {
	Time                time.Duration // KeepAlive间隔时间
	Timeout             time.Duration // KeepAlive超时时间
	PermitWithoutStream bool          // 即使没有活跃流也发送KeepAlive
}

// NewClient 创建新的gRPC客户端
func NewClient(config ClientConfig) (*Client, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("address is required")
	}

	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 如果使用服务发现，修改地址格式并注册 resolver
	address := config.Address
	if config.ServiceDiscovery != nil {
		// 检测 ServiceDiscovery 类型以确定 scheme
		scheme := StaticScheme
		switch config.ServiceDiscovery.(type) {
		case *EtcdResolver:
			scheme = EtcdScheme
		case *StaticResolver:
			scheme = StaticScheme
		}
		
		// 如果地址不包含 scheme，添加 scheme
		if !containsScheme(address) {
			// 使用服务发现时，地址应该是服务名称
			// 格式：scheme://service-name
			address = fmt.Sprintf("%s://%s", scheme, address)
		} else {
			// 如果地址包含 scheme，提取它
			extractedScheme := extractScheme(address)
			if extractedScheme != "" {
				scheme = extractedScheme
			}
		}
		
		// 注册 resolver
		RegisterResolver(scheme, config.ServiceDiscovery)
	}

	client := &Client{
		address: address,
		timeout: config.Timeout,
		ctx:     ctx,
		cancel:  cancel,
	}

	// 构建 DialOption
	options := []grpc.DialOption{}

	// 添加传输安全选项
	if config.Insecure {
		options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if config.TLS != nil {
		var creds credentials.TransportCredentials
		var err error

		if config.TLS.CAFile != "" {
			// 使用CA证书验证服务器
			serverName := config.TLS.ServerName
			if serverName == "" {
				serverName = "localhost"
			}
			creds, err = credentials.NewClientTLSFromFile(config.TLS.CAFile, serverName)
		} else {
			// 使用系统默认证书
			creds = credentials.NewTLS(nil)
		}

		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
		}

		options = append(options, grpc.WithTransportCredentials(creds))
	} else {
		// 默认使用非安全连接
		options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 添加KeepAlive配置
	if config.KeepAlive != nil {
		options = append(options, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                config.KeepAlive.Time,
			Timeout:             config.KeepAlive.Timeout,
			PermitWithoutStream: config.KeepAlive.PermitWithoutStream,
		}))
	}

	// 构建拦截器链
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		ClientLoggingInterceptor(),
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		ClientStreamLoggingInterceptor(),
	}

	// 如果启用了 OpenTelemetry tracing，添加 tracing 拦截器
	if tracing.IsEnabled() {
		unaryInterceptors = append([]grpc.UnaryClientInterceptor{tracing.UnaryClientInterceptor()}, unaryInterceptors...)
		streamInterceptors = append([]grpc.StreamClientInterceptor{tracing.StreamClientInterceptor()}, streamInterceptors...)
	}

	// 添加默认拦截器（日志、链路追踪）
	options = append(options, grpc.WithChainUnaryInterceptor(unaryInterceptors...))
	// 添加流式拦截器
	options = append(options, grpc.WithChainStreamInterceptor(streamInterceptors...))

	// 添加负载均衡策略
	if config.LoadBalancing != "" {
		options = append(options, GetLoadBalancingOption(config.LoadBalancing))
	} else {
		// 如果使用服务发现，默认使用轮询策略
		if config.ServiceDiscovery != nil {
			options = append(options, GetLoadBalancingOption(PolicyRoundRobin))
		}
	}

	// 添加自定义选项
	options = append(options, config.Options...)

	client.options = options

	return client, nil
}

// Connect 连接到gRPC服务器
func (c *Client) Connect(ctx context.Context) error {
	if c.conn != nil {
		return fmt.Errorf("client already connected")
	}

	// 创建带超时的context
	connectCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// 连接服务器
	conn, err := grpc.DialContext(connectCtx, c.address, c.options...)
	if err != nil {
		logger.Error(ctx, "Failed to connect to gRPC server: address=%s", c.address, err)
		return fmt.Errorf("failed to connect to %s: %w", c.address, err)
	}

	c.conn = conn
	logger.Info(ctx, "gRPC client connected: address=%s", c.address)

	return nil
}

// ConnectWithContext 使用context连接到gRPC服务器
func (c *Client) ConnectWithContext(ctx context.Context) error {
	return c.Connect(ctx)
}

// GetConn 获取底层连接
func (c *Client) GetConn() *grpc.ClientConn {
	return c.conn
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	if c.conn == nil {
		return false
	}
	state := c.conn.GetState()
	return state.String() == "READY"
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}

	ctx := context.Background()
	err := c.conn.Close()
	if err != nil {
		logger.Error(ctx, "Failed to close gRPC client connection: address=%s", c.address, err)
		return err
	}

	logger.Info(ctx, "gRPC client connection closed: address=%s", c.address)
	c.conn = nil

	if c.cancel != nil {
		c.cancel()
	}

	return nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context, service string) (*grpc_health_v1.HealthCheckResponse, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("client not connected")
	}

	healthClient := grpc_health_v1.NewHealthClient(c.conn)

	req := &grpc_health_v1.HealthCheckRequest{
		Service: service,
	}

	resp, err := healthClient.Check(ctx, req)
	if err != nil {
		logger.Error(ctx, "Health check failed: service=%s, address=%s", service, c.address, err)
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	return resp, nil
}

// GetAddress 获取服务器地址
func (c *Client) GetAddress() string {
	return c.address
}

// WithTimeout 创建带超时的context
func (c *Client) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// WithDeadline 创建带截止时间的context
func (c *Client) WithDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, deadline)
}

// containsScheme 检查地址是否包含 scheme
func containsScheme(address string) bool {
	for i := 0; i < len(address); i++ {
		if address[i] == ':' {
			if i+2 < len(address) && address[i+1] == '/' && address[i+2] == '/' {
				return true
			}
			return false
		}
		if address[i] == '/' {
			return false
		}
	}
	return false
}

// extractScheme 从地址中提取 scheme
func extractScheme(address string) string {
	for i := 0; i < len(address); i++ {
		if address[i] == ':' {
			if i+2 < len(address) && address[i+1] == '/' && address[i+2] == '/' {
				return address[:i]
			}
			return ""
		}
		if address[i] == '/' {
			return ""
		}
	}
	return ""
}
