package tracing

// Config 链路追踪配置
type Config struct {
	// 是否启用链路追踪
	Enabled bool `json:"enabled" yaml:"enabled" toml:"enabled"`
	// 服务名称（用于标识服务）
	ServiceName string `json:"serviceName" yaml:"serviceName" toml:"serviceName"`
	// 服务版本
	ServiceVersion string `json:"serviceVersion" yaml:"serviceVersion" toml:"serviceVersion"`
	// 环境名称（如：dev、staging、prod）
	Environment string `json:"environment" yaml:"environment" toml:"environment"`
	// Jaeger 配置（已废弃，建议使用 OTLP）
	// Deprecated: 建议使用 OTLP 配置
	Jaeger JaegerConfig `json:"jaeger" yaml:"jaeger" toml:"jaeger"`
	// OTLP 配置（推荐使用，Jaeger 支持 OTLP）
	OTLP OTLPConfig `json:"otlp" yaml:"otlp" toml:"otlp"`
	// 采样率（0.0-1.0，1.0 表示采样所有请求）
	SamplingRate float64 `json:"samplingRate" yaml:"samplingRate" toml:"samplingRate"`
}

// OTLPConfig OTLP 配置（推荐使用）
type OTLPConfig struct {
	// 是否启用 OTLP 上传
	Enabled bool `json:"enabled" yaml:"enabled" toml:"enabled"`
	// OTLP 端点（如：http://localhost:4317 或 http://localhost:4318）
	// gRPC 默认端口：4317，HTTP 默认端口：4318
	Endpoint string `json:"endpoint" yaml:"endpoint" toml:"endpoint"`
	// 是否使用 gRPC（默认 false，使用 HTTP）
	Insecure bool `json:"insecure" yaml:"insecure" toml:"insecure"`
	// 是否使用 gRPC（如果为 true，使用 gRPC，否则使用 HTTP）
	UseGRPC bool `json:"useGRPC" yaml:"useGRPC" toml:"useGRPC"`
	// 请求头（用于认证等）
	Headers map[string]string `json:"headers" yaml:"headers" toml:"headers"`
}

// JaegerConfig Jaeger 配置
type JaegerConfig struct {
	// 是否启用 Jaeger 上传
	Enabled bool `json:"enabled" yaml:"enabled" toml:"enabled"`
	// Jaeger Agent 地址（如：localhost:6831）
	AgentHost string `json:"agentHost" yaml:"agentHost" toml:"agentHost"`
	// Jaeger Agent 端口（UDP，默认 6831）
	AgentPort int `json:"agentPort" yaml:"agentPort" toml:"agentPort"`
	// Jaeger Collector 端点（HTTP，如：http://localhost:14268/api/traces）
	// 如果设置了此值，将使用 HTTP 方式上传，否则使用 UDP Agent
	CollectorEndpoint string `json:"collectorEndpoint" yaml:"collectorEndpoint" toml:"collectorEndpoint"`
	// 用户名（如果 Collector 需要认证）
	Username string `json:"username" yaml:"username" toml:"username"`
	// 密码（如果 Collector 需要认证）
	Password string `json:"password" yaml:"password" toml:"password"`
}
