package gorm

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/team-dandelion/quickgo/logger"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// Client GORM 客户端封装
type Client struct {
	name   string
	db     *gorm.DB
	config *GormConfig
}

// NewClient 创建 GORM 客户端
func NewClient(config *GormConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("gorm config is nil")
	}

	if config.Name == "" {
		return nil, fmt.Errorf("database name is required")
	}

	ctx := context.Background()
	logger.Info(ctx, "Initializing GORM client: name=%s, type=%s", config.Name, config.Master.Type)

	// 构建主库 DSN
	masterDSN, err := buildDSN(config.Master)
	if err != nil {
		return nil, fmt.Errorf("failed to build master DSN: %w", err)
	}

	dialector, err := newDialector(config.Master.Type, masterDSN)
	if err != nil {
		return nil, err
	}

	// 打开主库连接
	db, err := gorm.Open(dialector, newGormConfig(config))
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection (check database is running and accessible): %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if config.MaxIdleConn > 0 {
		sqlDB.SetMaxIdleConns(config.MaxIdleConn)
	}
	if config.MaxOpenConn > 0 {
		sqlDB.SetMaxOpenConns(config.MaxOpenConn)
	}

	// 解析并设置连接最大生存时间
	if config.ConnMaxLifetime != "" {
		connMaxLifetime, err := time.ParseDuration(config.ConnMaxLifetime)
		if err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("failed to parse ConnMaxLifetime %s: %w", config.ConnMaxLifetime, err)
		}
		if connMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(connMaxLifetime)
		}
	}

	// 解析并设置连接最大空闲时间
	if config.ConnMaxIdleTime != "" {
		connMaxIdleTime, err := time.ParseDuration(config.ConnMaxIdleTime)
		if err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("failed to parse ConnMaxIdleTime %s: %w", config.ConnMaxIdleTime, err)
		}
		if connMaxIdleTime > 0 {
			sqlDB.SetConnMaxIdleTime(connMaxIdleTime)
		}
	}

	// 测试连接（使用带超时的 context，确保不会无限等待）
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		// 连接失败，关闭已创建的连接
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database (connection test failed): %w", err)
	}

	// 如果配置了从库，设置读写分离
	// 注意：从库连接失败也会导致服务无法启动
	if len(config.Slaves) > 0 {
		logger.Info(ctx, "Configuring read replicas: name=%s, count=%d", config.Name, len(config.Slaves))

		var slaveDialectors []gorm.Dialector
		for i, slave := range config.Slaves {
			slaveDSN, err := buildSlaveDSN(config.Master.Type, slave)
			if err != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("failed to build slave[%d] DSN: %w", i, err)
			}

			probeDialector, err := newDialector(config.Master.Type, slaveDSN)
			if err != nil {
				sqlDB.Close()
				return nil, err
			}

			// 测试从库连接（确保从库可用）
			slaveDB, err := gorm.Open(probeDialector, newGormConfig(config))
			if err != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("failed to connect to slave[%d] (read replica connection failed): %w", i, err)
			}

			slaveSQLDB, err := slaveDB.DB()
			if err != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("failed to get slave[%d] sql.DB: %w", i, err)
			}

			// 测试从库连接
			slavePingCtx, slavePingCancel := context.WithTimeout(context.Background(), 5*time.Second)
			pingErr := slaveSQLDB.PingContext(slavePingCtx)
			slavePingCancel()
			closeErr := slaveSQLDB.Close()
			if pingErr != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("failed to ping slave[%d] (read replica connection test failed): %w", i, pingErr)
			}
			if closeErr != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("failed to close slave[%d] probe connection: %w", i, closeErr)
			}

			slaveDialector, err := newDialector(config.Master.Type, slaveDSN)
			if err != nil {
				sqlDB.Close()
				return nil, err
			}

			// 从库连接成功，添加到列表
			slaveDialectors = append(slaveDialectors, slaveDialector)
			logger.Info(ctx, "Slave[%d] connected successfully: name=%s", i, config.Name)
		}

		// 配置读写分离
		err = db.Use(dbresolver.Register(dbresolver.Config{
			Replicas:          slaveDialectors,
			Policy:            dbresolver.RandomPolicy{},
			TraceResolverMode: true,
		}))
		if err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("failed to register db resolver: %w", err)
		}

		logger.Info(ctx, "Read replicas configured successfully: name=%s, count=%d", config.Name, len(slaveDialectors))
	}

	logger.Info(ctx, "GORM client initialized successfully: name=%s", config.Name)

	return &Client{
		name:   config.Name,
		db:     db,
		config: config,
	}, nil
}

func newGormConfig(config *GormConfig) *gorm.Config {
	return &gorm.Config{
		Logger: newLogger(config),
	}
}

func newDialector(dbType DatabaseType, dsn string) (gorm.Dialector, error) {
	switch dbType {
	case DatabaseTypeMySQL:
		return gormmysql.Open(dsn), nil
	case DatabaseTypePostgreSQL:
		return postgres.Open(dsn), nil
	case DatabaseTypeSQLite:
		return sqlite.Open(dsn), nil
	case DatabaseTypeSQLServer:
		return sqlserver.Open(dsn), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// GetDB 获取 GORM DB 实例
func (c *Client) GetDB() *gorm.DB {
	return c.db
}

// GetName 获取数据库名称
func (c *Client) GetName() string {
	return c.name
}

// Close 关闭数据库连接
func (c *Client) Close() error {
	if c.db == nil {
		return nil
	}

	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}

	ctx := context.Background()
	logger.Info(ctx, "Closing GORM client: name=%s", c.name)

	return sqlDB.Close()
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	if c.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// buildDSN 构建主库 DSN
func buildDSN(master MasterConfig) (string, error) {
	// 如果提供了 DSN，直接使用
	if master.DSN != "" {
		return master.DSN, nil
	}

	// 根据数据库类型构建 DSN
	switch master.Type {
	case DatabaseTypeMySQL:
		return buildMySQLDSN(master), nil
	case DatabaseTypePostgreSQL:
		return buildPostgreSQLDSN(master), nil
	case DatabaseTypeSQLite:
		if master.Database == "" {
			return "", fmt.Errorf("database path is required for SQLite")
		}
		return master.Database, nil
	case DatabaseTypeSQLServer:
		return buildSQLServerDSN(master), nil
	default:
		return "", fmt.Errorf("unsupported database type: %s", master.Type)
	}
}

// buildMySQLDSN 构建 MySQL DSN
func buildMySQLDSN(master MasterConfig) string {
	params := make(map[string]string)
	if master.Charset != "" {
		params["charset"] = master.Charset
	} else {
		params["charset"] = "utf8mb4"
	}

	parseTime := true
	if master.Timezone != "" {
		params["loc"] = master.Timezone
	} else {
		params["loc"] = "Local"
	}

	// 添加其他参数
	for k, v := range master.Params {
		if k == "parseTime" {
			parseTime = v == "true" || v == "True" || v == "1"
			continue
		}
		params[k] = v
	}

	cfg := mysqldriver.NewConfig()
	cfg.User = master.User
	cfg.Passwd = master.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(master.Host, fmt.Sprintf("%d", master.Port))
	cfg.DBName = master.Database
	cfg.ParseTime = parseTime
	cfg.Params = params
	return cfg.FormatDSN()
}

func userInfo(user, password string) *url.Userinfo {
	if user == "" && password == "" {
		return nil
	}
	if password == "" {
		return url.User(user)
	}
	return url.UserPassword(user, password)
}

// buildPostgreSQLDSN 构建 PostgreSQL DSN
func buildPostgreSQLDSN(master MasterConfig) string {
	values := make(url.Values)
	values.Set("sslmode", "disable")

	if master.SSLMode != "" {
		values.Set("sslmode", master.SSLMode)
	}
	for k, v := range master.Params {
		values.Set(k, v)
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     userInfo(master.User, master.Password),
		Host:     net.JoinHostPort(master.Host, fmt.Sprintf("%d", master.Port)),
		Path:     "/" + master.Database,
		RawQuery: values.Encode(),
	}
	return u.String()
}

// buildSQLServerDSN 构建 SQL Server DSN
func buildSQLServerDSN(master MasterConfig) string {
	values := make(url.Values)
	values.Set("database", master.Database)
	for k, v := range master.Params {
		values.Set(k, v)
	}
	u := url.URL{
		Scheme:   "sqlserver",
		User:     userInfo(master.User, master.Password),
		Host:     net.JoinHostPort(master.Host, fmt.Sprintf("%d", master.Port)),
		RawQuery: values.Encode(),
	}
	return u.String()
}

// buildSlaveDSN 构建从库 DSN
func buildSlaveDSN(dbType DatabaseType, slave SlaveConfig) (string, error) {
	switch dbType {
	case DatabaseTypeMySQL:
		return buildMySQLSlaveDSN(slave), nil
	case DatabaseTypePostgreSQL:
		return buildPostgreSQLSlaveDSN(slave), nil
	case DatabaseTypeSQLite:
		return slave.Database, nil
	case DatabaseTypeSQLServer:
		return buildSQLServerSlaveDSN(slave), nil
	default:
		return "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// buildMySQLSlaveDSN 构建 MySQL 从库 DSN
func buildMySQLSlaveDSN(slave SlaveConfig) string {
	params := make(map[string]string)
	if slave.Charset != "" {
		params["charset"] = slave.Charset
	} else {
		params["charset"] = "utf8mb4"
	}

	parseTime := true
	if slave.Timezone != "" {
		params["loc"] = slave.Timezone
	} else {
		params["loc"] = "Local"
	}

	// 添加其他参数
	for k, v := range slave.Params {
		if k == "parseTime" {
			parseTime = v == "true" || v == "True" || v == "1"
			continue
		}
		params[k] = v
	}

	cfg := mysqldriver.NewConfig()
	cfg.User = slave.User
	cfg.Passwd = slave.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(slave.Host, fmt.Sprintf("%d", slave.Port))
	cfg.DBName = slave.Database
	cfg.ParseTime = parseTime
	cfg.Params = params
	return cfg.FormatDSN()
}

// buildPostgreSQLSlaveDSN 构建 PostgreSQL 从库 DSN
func buildPostgreSQLSlaveDSN(slave SlaveConfig) string {
	values := make(url.Values)
	values.Set("sslmode", "disable")

	if slave.SSLMode != "" {
		values.Set("sslmode", slave.SSLMode)
	}

	// 添加其他参数
	for k, v := range slave.Params {
		values.Set(k, v)
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     userInfo(slave.User, slave.Password),
		Host:     net.JoinHostPort(slave.Host, fmt.Sprintf("%d", slave.Port)),
		Path:     "/" + slave.Database,
		RawQuery: values.Encode(),
	}
	return u.String()
}

// buildSQLServerSlaveDSN 构建 SQL Server 从库 DSN
func buildSQLServerSlaveDSN(slave SlaveConfig) string {
	values := make(url.Values)
	values.Set("database", slave.Database)
	for k, v := range slave.Params {
		values.Set(k, v)
	}
	u := url.URL{
		Scheme:   "sqlserver",
		User:     userInfo(slave.User, slave.Password),
		Host:     net.JoinHostPort(slave.Host, fmt.Sprintf("%d", slave.Port)),
		RawQuery: values.Encode(),
	}
	return u.String()
}
