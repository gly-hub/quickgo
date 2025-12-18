package quickgo

import (
	"context"
	"time"

	"github.com/team-dandelion/quickgo/db/gorm"
	"github.com/team-dandelion/quickgo/db/mongodb"
	"github.com/team-dandelion/quickgo/db/redis"
	"github.com/team-dandelion/quickgo/logger"
)

// ExampleDatabaseUsage 数据库使用示例
func ExampleDatabaseUsage() {
	// 1. 初始化配置（从配置文件加载）
	InitConfig("local")

	// 2. 加载配置到结构体
	var config = struct {
		AppConfig        AppConfig                  `json:"app" yaml:"app"`
		LoggerConfig     LoggerConfig               `json:"logger" yaml:"logger"`
		GormConfig       gorm.GormManagerConfig     `json:"gorm" yaml:"gorm"`
		MongoDBConfig    mongodb.MongoManagerConfig `json:"mongodb" yaml:"mongodb"`
		RedisConfig      redis.RedisManagerConfig   `json:"redis" yaml:"redis"`
		HTTPServerConfig HTTPServerConfig           `json:"httpServer" yaml:"httpServer"`
	}{}
	LoadCustomConfig(&config)

	// 3. 创建框架实例，使用 Option 模式显式指定需要初始化的组件
	app, err := NewFramework(
		ConfigOptionWithApp(config.AppConfig),
		ConfigOptionWithLogger(config.LoggerConfig),
		ConfigOptionWithGorm(&config.GormConfig),
		ConfigOptionWithMongoDB(&config.MongoDBConfig),
		ConfigOptionWithRedis(&config.RedisConfig),
		ConfigOptionWithHTTPServer(&config.HTTPServerConfig),
	)
	if err != nil {
		panic(err)
	}

	// 4. 初始化所有组件
	if err := app.Init(); err != nil {
		panic(err)
	}

	ctx := context.Background()

	// ==================== GORM 使用示例 ====================
	if app.GormManager() != nil {
		// 获取主数据库（user-db）
		userDB, err := app.GormManager().GetDB("user-db")
		if err != nil {
			logger.Error(ctx, "Failed to get user-db: %v", err)
		} else {
			logger.Info(ctx, "Got user-db connection")
			// 使用 GORM 进行数据库操作
			// userDB.AutoMigrate(&User{})
			// userDB.Create(&User{Name: "test"})
			_ = userDB // 示例中未实际使用
		}

		// 获取订单数据库（order-db）
		orderDB, err := app.GormManager().GetDB("order-db")
		if err != nil {
			logger.Error(ctx, "Failed to get order-db: %v", err)
		} else {
			logger.Info(ctx, "Got order-db connection")
			// 使用 GORM 进行数据库操作
			// orderDB.AutoMigrate(&Order{})
			_ = orderDB // 示例中未实际使用
		}

		// 列出所有已注册的 GORM 客户端
		clients := app.GormManager().ListClients()
		logger.Info(ctx, "GORM clients: %v", clients)

		// 健康检查
		if err := app.GormManager().HealthCheck(ctx); err != nil {
			logger.Error(ctx, "GORM health check failed: %v", err)
		} else {
			logger.Info(ctx, "GORM health check passed")
		}
	}

	// ==================== MongoDB 使用示例 ====================
	if app.MongoManager() != nil {
		// 获取主数据库（main-mongo）
		mainDB, err := app.MongoManager().GetDB("main-mongo")
		if err != nil {
			logger.Error(ctx, "Failed to get main-mongo: %v", err)
		} else {
			logger.Info(ctx, "Got main-mongo connection")
			// 使用 MongoDB 进行数据库操作
			// collection := mainDB.Collection("users")
			// result, _ := collection.InsertOne(ctx, bson.M{"name": "test"})
			_ = mainDB // 示例中未实际使用
		}

		// 获取日志数据库（log-mongo）
		logDB, err := app.MongoManager().GetDB("log-mongo")
		if err != nil {
			logger.Error(ctx, "Failed to get log-mongo: %v", err)
		} else {
			logger.Info(ctx, "Got log-mongo connection")
			_ = logDB // 示例中未实际使用
		}

		// 列出所有已注册的 MongoDB 客户端
		clients := app.MongoManager().ListClients()
		logger.Info(ctx, "MongoDB clients: %v", clients)

		// 健康检查
		if err := app.MongoManager().HealthCheck(ctx); err != nil {
			logger.Error(ctx, "MongoDB health check failed: %v", err)
		} else {
			logger.Info(ctx, "MongoDB health check passed")
		}
	}

	// ==================== Redis 使用示例 ====================
	if app.RedisManager() != nil {
		// 获取缓存 Redis（cache-redis）
		cacheClient, err := app.RedisManager().GetClient("cache-redis")
		if err != nil {
			logger.Error(ctx, "Failed to get cache-redis: %v", err)
		} else {
			logger.Info(ctx, "Got cache-redis connection")
			// 使用 Redis 进行操作
			redisClient := cacheClient.GetClient()
			redisClient.Set(ctx, "key1", "value1", time.Hour)
			val, _ := redisClient.Get(ctx, "key1").Result()
			logger.Info(ctx, "Redis get result: %s", val)
		}

		// 获取会话 Redis（session-redis）
		sessionClient, err := app.RedisManager().GetClient("session-redis")
		if err != nil {
			logger.Error(ctx, "Failed to get session-redis: %v", err)
		} else {
			logger.Info(ctx, "Got session-redis connection")
			// 使用 Redis 进行操作
			redisClient := sessionClient.GetClient()
			redisClient.Set(ctx, "session:user123", "session_data", 30*time.Minute)
		}

		// 列出所有已注册的 Redis 客户端
		clients := app.RedisManager().ListClients()
		logger.Info(ctx, "Redis clients: %v", clients)

		// 健康检查
		if err := app.RedisManager().HealthCheck(ctx); err != nil {
			logger.Error(ctx, "Redis health check failed: %v", err)
		} else {
			logger.Info(ctx, "Redis health check passed")
		}
	}

	// 5. 启动所有组件
	if err := app.Start(); err != nil {
		panic(err)
	}

	// 6. 等待中断信号（优雅关闭）
	app.Wait()
}

// ExampleGormWithMasterSlave GORM 主从配置示例
func ExampleGormWithMasterSlave() {
	// 配置示例：支持主从读写分离
	gormConfig := &gorm.GormManagerConfig{
		Databases: []gorm.GormConfig{
			{
				Name: "user-db",
				Master: gorm.MasterConfig{
					Type:     gorm.DatabaseTypeMySQL,
					Host:     "127.0.0.1",
					Port:     3306,
					User:     "root",
					Password: "password",
					Database: "user_db",
					Charset:  "utf8mb4",
					Timezone: "Local",
				},
				// 配置从库（用于读操作）
				Slaves: []gorm.SlaveConfig{
					{
						Host:     "127.0.0.1",
						Port:     3307,
						User:     "root",
						Password: "password",
						Database: "user_db",
						Charset:  "utf8mb4",
						Timezone: "Local",
					},
					{
						Host:     "127.0.0.1",
						Port:     3308,
						User:     "root",
						Password: "password",
						Database: "user_db",
						Charset:  "utf8mb4",
						Timezone: "Local",
					},
				},
				MaxIdleConn:     10,
				MaxOpenConn:     100,
				ConnMaxLifetime: "30m",
				ConnMaxIdleTime: "30m",
				EnableLog:       true,
				LogLevel:        "info",
				SlowThreshold:   200, // 200ms
			},
		},
	}

	app, _ := NewFramework(
		ConfigOptionWithApp(AppConfig{Name: "test", Version: "1.0.0"}),
		ConfigOptionWithLogger(LoggerConfig{Enabled: true, Level: "info"}),
		ConfigOptionWithGorm(gormConfig),
	)

	if err := app.Init(); err != nil {
		panic(err)
	}

	// 使用数据库（GORM 会自动路由读操作到从库）
	db, _ := app.GormManager().GetDB("user-db")
	// 写操作会使用主库
	// db.Create(&User{Name: "test"})
	// 读操作会自动路由到从库（随机选择）
	// var user User
	// db.First(&user, 1)
	_ = db
}

// ExampleMultipleDatabases 多数据库实例示例
func ExampleMultipleDatabases() {
	// 配置多个 MySQL 实例
	gormConfig := &gorm.GormManagerConfig{
		Databases: []gorm.GormConfig{
			{
				Name: "user-db",
				Master: gorm.MasterConfig{
					Type:     gorm.DatabaseTypeMySQL,
					Host:     "127.0.0.1",
					Port:     3306,
					User:     "root",
					Password: "password",
					Database: "user_db",
				},
			},
			{
				Name: "order-db",
				Master: gorm.MasterConfig{
					Type:     gorm.DatabaseTypeMySQL,
					Host:     "127.0.0.1",
					Port:     3306,
					User:     "root",
					Password: "password",
					Database: "order_db",
				},
			},
			{
				Name: "product-db",
				Master: gorm.MasterConfig{
					Type:     gorm.DatabaseTypePostgreSQL,
					Host:     "127.0.0.1",
					Port:     5432,
					User:     "postgres",
					Password: "password",
					Database: "product_db",
					SSLMode:  "disable",
				},
			},
		},
	}

	// 配置多个 Redis 实例
	redisConfig := &redis.RedisManagerConfig{
		Databases: []redis.RedisConfig{
			{
				Name:     "cache-redis",
				Host:     "127.0.0.1",
				Port:     6379,
				Password: "",
				DB:       0,
			},
			{
				Name:     "session-redis",
				Host:     "127.0.0.1",
				Port:     6380,
				Password: "",
				DB:       0,
			},
		},
	}

	// 配置多个 MongoDB 实例
	mongoConfig := &mongodb.MongoManagerConfig{
		Databases: []mongodb.MongoConfig{
			{
				Name:     "main-mongo",
				Host:     "127.0.0.1",
				Port:     27017,
				Database: "main_db",
			},
			{
				Name:     "log-mongo",
				Host:     "127.0.0.1",
				Port:     27018,
				Database: "log_db",
			},
		},
	}

	app, _ := NewFramework(
		ConfigOptionWithApp(AppConfig{Name: "test", Version: "1.0.0"}),
		ConfigOptionWithLogger(LoggerConfig{Enabled: true, Level: "info"}),
		ConfigOptionWithGorm(gormConfig),
		ConfigOptionWithRedis(redisConfig),
		ConfigOptionWithMongoDB(mongoConfig),
	)

	if err := app.Init(); err != nil {
		panic(err)
	}

	ctx := context.Background()

	// 使用不同的数据库实例
	userDB, _ := app.GormManager().GetDB("user-db")
	orderDB, _ := app.GormManager().GetDB("order-db")
	productDB, _ := app.GormManager().GetDB("product-db")

	cacheRedis, _ := app.RedisManager().GetClient("cache-redis")
	sessionRedis, _ := app.RedisManager().GetClient("session-redis")

	mainMongo, _ := app.MongoManager().GetDB("main-mongo")
	logMongo, _ := app.MongoManager().GetDB("log-mongo")

	logger.Info(ctx, "Multiple databases initialized: user-db=%v, order-db=%v, product-db=%v",
		userDB != nil, orderDB != nil, productDB != nil)
	logger.Info(ctx, "Multiple Redis initialized: cache=%v, session=%v",
		cacheRedis != nil, sessionRedis != nil)
	logger.Info(ctx, "Multiple MongoDB initialized: main=%v, log=%v",
		mainMongo != nil, logMongo != nil)
}

// ExampleDynamicRegister 动态注册数据库示例
func ExampleDynamicRegister() {
	app, _ := NewFramework(
		ConfigOptionWithApp(AppConfig{Name: "test", Version: "1.0.0"}),
		ConfigOptionWithLogger(LoggerConfig{Enabled: true, Level: "info"}),
	)

	if err := app.Init(); err != nil {
		panic(err)
	}

	ctx := context.Background()

	// 动态注册 GORM 客户端
	gormConfig := &gorm.GormConfig{
		Name: "dynamic-db",
		Master: gorm.MasterConfig{
			Type:     gorm.DatabaseTypeMySQL,
			Host:     "127.0.0.1",
			Port:     3306,
			User:     "root",
			Password: "password",
			Database: "dynamic_db",
		},
	}

	if err := app.GormManager().RegisterClient(gormConfig); err != nil {
		logger.Error(ctx, "Failed to register GORM client: %v", err)
	} else {
		logger.Info(ctx, "GORM client registered dynamically")
		db, _ := app.GormManager().GetDB("dynamic-db")
		_ = db
	}

	// 动态注册 Redis 客户端
	redisConfig := &redis.RedisConfig{
		Name:     "dynamic-redis",
		Host:     "127.0.0.1",
		Port:     6379,
		Password: "",
		DB:       0,
	}

	if err := app.RedisManager().RegisterClient(redisConfig); err != nil {
		logger.Error(ctx, "Failed to register Redis client: %v", err)
	} else {
		logger.Info(ctx, "Redis client registered dynamically")
		client, _ := app.RedisManager().GetClient("dynamic-redis")
		_ = client
	}

	// 动态注册 MongoDB 客户端
	mongoConfig := &mongodb.MongoConfig{
		Name:     "dynamic-mongo",
		Host:     "127.0.0.1",
		Port:     27017,
		Database: "dynamic_db",
	}

	if err := app.MongoManager().RegisterClient(mongoConfig); err != nil {
		logger.Error(ctx, "Failed to register MongoDB client: %v", err)
	} else {
		logger.Info(ctx, "MongoDB client registered dynamically")
		db, _ := app.MongoManager().GetDB("dynamic-mongo")
		_ = db
	}
}
