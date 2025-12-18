package mongodb

// MongoConfig MongoDB 配置
type MongoConfig struct {
	// 数据库名称（用于多实例管理）
	Name string `json:"name" yaml:"name" toml:"name"`
	// 连接 URI（如果提供，则忽略其他连接参数）
	URI string `json:"uri" yaml:"uri" toml:"uri"`
	// 主机地址（不使用 URI 时）
	Host string `json:"host" yaml:"host" toml:"host"`
	// 端口（不使用 URI 时）
	Port int `json:"port" yaml:"port" toml:"port"`
	// 用户名（不使用 URI 时）
	Username string `json:"username" yaml:"username" toml:"username"`
	// 密码（不使用 URI 时）
	Password string `json:"password" yaml:"password" toml:"password"`
	// 数据库名（不使用 URI 时）
	Database string `json:"database" yaml:"database" toml:"database"`
	// 认证数据库（不使用 URI 时）
	AuthSource string `json:"authSource" yaml:"authSource" toml:"authSource"`
	// 连接池配置
	MaxPoolSize     uint64 `json:"maxPoolSize" yaml:"maxPoolSize" toml:"maxPoolSize"`         // 最大连接池大小
	MinPoolSize     uint64 `json:"minPoolSize" yaml:"minPoolSize" toml:"minPoolSize"`         // 最小连接池大小
	MaxConnIdleTime string `json:"maxConnIdleTime" yaml:"maxConnIdleTime" toml:"maxConnIdleTime"` // 连接最大空闲时间（如：30m、1h）
	ConnectTimeout  string `json:"connectTimeout" yaml:"connectTimeout" toml:"connectTimeout"`     // 连接超时时间（如：10s、30s）
	SocketTimeout   string `json:"socketTimeout" yaml:"socketTimeout" toml:"socketTimeout"`       // Socket 超时时间（如：30s、1m）
	// 其他选项
	Options map[string]string `json:"options" yaml:"options" toml:"options"`
}

// MongoManagerConfig MongoDB 管理器配置（支持多个数据库实例）
type MongoManagerConfig struct {
	// 数据库配置列表
	Databases []MongoConfig `json:"databases" yaml:"databases" toml:"databases"`
}

