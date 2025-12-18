package gorm

// DatabaseType 数据库类型
type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypeSQLServer  DatabaseType = "sqlserver"
)

// MasterConfig 主库配置
type MasterConfig struct {
	// 数据库类型：mysql, postgres, sqlite, sqlserver
	Type DatabaseType `json:"type" yaml:"type" toml:"type"`
	// DSN 连接字符串（如果提供，则忽略其他连接参数）
	DSN string `json:"dsn" yaml:"dsn" toml:"dsn"`
	// 主机地址（不使用 DSN 时）
	Host string `json:"host" yaml:"host" toml:"host"`
	// 端口（不使用 DSN 时）
	Port int `json:"port" yaml:"port" toml:"port"`
	// 用户名（不使用 DSN 时）
	User string `json:"user" yaml:"user" toml:"user"`
	// 密码（不使用 DSN 时）
	Password string `json:"password" yaml:"password" toml:"password"`
	// 数据库名（不使用 DSN 时）
	Database string `json:"database" yaml:"database" toml:"database"`
	// 字符集（MySQL 使用）
	Charset string `json:"charset" yaml:"charset" toml:"charset"`
	// 时区（MySQL 使用）
	Timezone string `json:"timezone" yaml:"timezone" toml:"timezone"`
	// SSL 模式（PostgreSQL 使用）
	SSLMode string `json:"sslMode" yaml:"sslMode" toml:"sslMode"`
	// 其他连接参数
	Params map[string]string `json:"params" yaml:"params" toml:"params"`
}

// SlaveConfig 从库配置
type SlaveConfig struct {
	// 主机地址
	Host string `json:"host" yaml:"host" toml:"host"`
	// 端口
	Port int `json:"port" yaml:"port" toml:"port"`
	// 用户名
	User string `json:"user" yaml:"user" toml:"user"`
	// 密码
	Password string `json:"password" yaml:"password" toml:"password"`
	// 数据库名
	Database string `json:"database" yaml:"database" toml:"database"`
	// 字符集（MySQL 使用）
	Charset string `json:"charset" yaml:"charset" toml:"charset"`
	// 时区（MySQL 使用）
	Timezone string `json:"timezone" yaml:"timezone" toml:"timezone"`
	// SSL 模式（PostgreSQL 使用）
	SSLMode string `json:"sslMode" yaml:"sslMode" toml:"sslMode"`
	// 其他连接参数
	Params map[string]string `json:"params" yaml:"params" toml:"params"`
}

// GormConfig GORM 数据库配置
type GormConfig struct {
	// 数据库名称（用于多实例管理）
	Name string `json:"name" yaml:"name" toml:"name"`
	// 主库配置
	Master MasterConfig `json:"master" yaml:"master" toml:"master"`
	// 从库配置列表（可选，用于读写分离）
	Slaves []SlaveConfig `json:"slaves" yaml:"slaves" toml:"slaves"`
	// 连接池配置
	MaxIdleConn     int    `json:"maxIdleConn" yaml:"maxIdleConn" toml:"maxIdleConn"`         // 最大空闲连接数
	MaxOpenConn     int    `json:"maxOpenConn" yaml:"maxOpenConn" toml:"maxOpenConn"`         // 最大打开连接数
	ConnMaxLifetime string `json:"connMaxLifetime" yaml:"connMaxLifetime" toml:"connMaxLifetime"` // 连接最大生存时间（如：30m、1h）
	ConnMaxIdleTime string `json:"connMaxIdleTime" yaml:"connMaxIdleTime" toml:"connMaxIdleTime"` // 连接最大空闲时间（如：10m、30m）
	// GORM 配置
	LogLevel      string `json:"logLevel" yaml:"logLevel" toml:"logLevel"`           // 日志级别：silent, error, warn, info
	SlowThreshold int    `json:"slowThreshold" yaml:"slowThreshold" toml:"slowThreshold"` // 慢查询阈值（毫秒）
	// 是否启用日志
	EnableLog bool `json:"enableLog" yaml:"enableLog" toml:"enableLog"`
}

// GormManagerConfig GORM 管理器配置（支持多个数据库实例）
type GormManagerConfig struct {
	// 数据库配置列表
	Databases []GormConfig `json:"databases" yaml:"databases" toml:"databases"`
}

