package redis

// RedisConfig Redis 配置
type RedisConfig struct {
	// 数据库名称（用于多实例管理）
	Name string `json:"name" yaml:"name" toml:"name"`
	// 连接地址（如果提供，则忽略其他连接参数）
	Addr string `json:"addr" yaml:"addr" toml:"addr"`
	// 主机地址（不使用 Addr 时）
	Host string `json:"host" yaml:"host" toml:"host"`
	// 端口（不使用 Addr 时）
	Port int `json:"port" yaml:"port" toml:"port"`
	// 密码
	Password string `json:"password" yaml:"password" toml:"password"`
	// 数据库索引（0-15）
	DB int `json:"db" yaml:"db" toml:"db"`
	// 用户名（Redis 6.0+）
	Username string `json:"username" yaml:"username" toml:"username"`
	// 连接池配置
	PoolSize     int    `json:"poolSize" yaml:"poolSize" toml:"poolSize"`           // 连接池大小
	MinIdleConns int    `json:"minIdleConns" yaml:"minIdleConns" toml:"minIdleConns"` // 最小空闲连接数
	MaxConnAge   string `json:"maxConnAge" yaml:"maxConnAge" toml:"maxConnAge"`       // 连接最大生存时间（如：1h、30m）
	PoolTimeout  string `json:"poolTimeout" yaml:"poolTimeout" toml:"poolTimeout"`    // 获取连接超时时间（如：4s、5s）
	IdleTimeout  string `json:"idleTimeout" yaml:"idleTimeout" toml:"idleTimeout"`   // 空闲连接超时时间（如：5m、10m）
	DialTimeout  string `json:"dialTimeout" yaml:"dialTimeout" toml:"dialTimeout"`   // 连接超时时间（如：5s、10s）
	ReadTimeout  string `json:"readTimeout" yaml:"readTimeout" toml:"readTimeout"`    // 读取超时时间（如：3s、5s）
	WriteTimeout string `json:"writeTimeout" yaml:"writeTimeout" toml:"writeTimeout"` // 写入超时时间（如：3s、5s）
	// 是否启用 TLS
	TLS bool `json:"tls" yaml:"tls" toml:"tls"`
}

// RedisManagerConfig Redis 管理器配置（支持多个数据库实例）
type RedisManagerConfig struct {
	// 数据库配置列表
	Databases []RedisConfig `json:"databases" yaml:"databases" toml:"databases"`
}

