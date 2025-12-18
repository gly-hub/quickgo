package gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/team-dandelion/quickgo/logger"

	"gorm.io/driver/mysql"
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

	// 根据数据库类型选择驱动
	var dialector gorm.Dialector
	switch config.Master.Type {
	case DatabaseTypeMySQL:
		dialector = mysql.Open(masterDSN)
	case DatabaseTypePostgreSQL:
		dialector = postgres.Open(masterDSN)
	case DatabaseTypeSQLite:
		dialector = sqlite.Open(masterDSN)
	case DatabaseTypeSQLServer:
		dialector = sqlserver.Open(masterDSN)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Master.Type)
	}

	// GORM 配置
	gormConfig := &gorm.Config{
		Logger: newLogger(config),
	}

	// 打开主库连接
	db, err := gorm.Open(dialector, gormConfig)
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

			var slaveDialector gorm.Dialector
			switch config.Master.Type {
			case DatabaseTypeMySQL:
				slaveDialector = mysql.Open(slaveDSN)
			case DatabaseTypePostgreSQL:
				slaveDialector = postgres.Open(slaveDSN)
			case DatabaseTypeSQLite:
				slaveDialector = sqlite.Open(slaveDSN)
			case DatabaseTypeSQLServer:
				slaveDialector = sqlserver.Open(slaveDSN)
			default:
				sqlDB.Close()
				return nil, fmt.Errorf("unsupported database type: %s", config.Master.Type)
			}

			// 测试从库连接（确保从库可用）
			slaveDB, err := gorm.Open(slaveDialector, gormConfig)
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
			defer slavePingCancel()

			if err := slaveSQLDB.PingContext(slavePingCtx); err != nil {
				sqlDB.Close()
				slaveSQLDB.Close()
				return nil, fmt.Errorf("failed to ping slave[%d] (read replica connection test failed): %w", i, err)
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
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		master.User,
		master.Password,
		master.Host,
		master.Port,
		master.Database,
	)

	params := make(map[string]string)
	if master.Charset != "" {
		params["charset"] = master.Charset
	} else {
		params["charset"] = "utf8mb4"
	}

	if master.Timezone != "" {
		params["parseTime"] = "True"
		params["loc"] = master.Timezone
	} else {
		params["parseTime"] = "True"
		params["loc"] = "Local"
	}

	// 添加其他参数
	for k, v := range master.Params {
		params[k] = v
	}

	// 构建参数字符串
	paramStr := ""
	for k, v := range params {
		if paramStr != "" {
			paramStr += "&"
		}
		paramStr += fmt.Sprintf("%s=%s", k, v)
	}

	if paramStr != "" {
		dsn += "?" + paramStr
	}

	return dsn
}

// buildPostgreSQLDSN 构建 PostgreSQL DSN
func buildPostgreSQLDSN(master MasterConfig) string {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
		master.Host,
		master.Port,
		master.User,
		master.Password,
		master.Database,
	)

	if master.SSLMode != "" {
		dsn += " sslmode=" + master.SSLMode
	} else {
		dsn += " sslmode=disable"
	}

	// 添加其他参数
	for k, v := range master.Params {
		dsn += fmt.Sprintf(" %s=%s", k, v)
	}

	return dsn
}

// buildSQLServerDSN 构建 SQL Server DSN
func buildSQLServerDSN(master MasterConfig) string {
	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		master.User,
		master.Password,
		master.Host,
		master.Port,
		master.Database,
	)

	// 添加其他参数
	first := true
	for k, v := range master.Params {
		if first {
			dsn += "&"
			first = false
		} else {
			dsn += "&"
		}
		dsn += fmt.Sprintf("%s=%s", k, v)
	}

	return dsn
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
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		slave.User,
		slave.Password,
		slave.Host,
		slave.Port,
		slave.Database,
	)

	params := make(map[string]string)
	if slave.Charset != "" {
		params["charset"] = slave.Charset
	} else {
		params["charset"] = "utf8mb4"
	}

	if slave.Timezone != "" {
		params["parseTime"] = "True"
		params["loc"] = slave.Timezone
	} else {
		params["parseTime"] = "True"
		params["loc"] = "Local"
	}

	// 添加其他参数
	for k, v := range slave.Params {
		params[k] = v
	}

	// 构建参数字符串
	paramStr := ""
	for k, v := range params {
		if paramStr != "" {
			paramStr += "&"
		}
		paramStr += fmt.Sprintf("%s=%s", k, v)
	}

	if paramStr != "" {
		dsn += "?" + paramStr
	}

	return dsn
}

// buildPostgreSQLSlaveDSN 构建 PostgreSQL 从库 DSN
func buildPostgreSQLSlaveDSN(slave SlaveConfig) string {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
		slave.Host,
		slave.Port,
		slave.User,
		slave.Password,
		slave.Database,
	)

	if slave.SSLMode != "" {
		dsn += " sslmode=" + slave.SSLMode
	} else {
		dsn += " sslmode=disable"
	}

	// 添加其他参数
	for k, v := range slave.Params {
		dsn += fmt.Sprintf(" %s=%s", k, v)
	}

	return dsn
}

// buildSQLServerSlaveDSN 构建 SQL Server 从库 DSN
func buildSQLServerSlaveDSN(slave SlaveConfig) string {
	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		slave.User,
		slave.Password,
		slave.Host,
		slave.Port,
		slave.Database,
	)

	// 添加其他参数
	first := true
	for k, v := range slave.Params {
		if first {
			dsn += "&"
			first = false
		} else {
			dsn += "&"
		}
		dsn += fmt.Sprintf("%s=%s", k, v)
	}

	return dsn
}
