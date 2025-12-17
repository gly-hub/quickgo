package quickgo

// 配置加载使用示例

/*
// 示例 1: 使用新的 ConfigLoader API（推荐）
func ExampleNewConfigLoader() {
	// 创建配置加载器
	loader, err := NewConfigLoader("local")
	if err != nil {
		panic(err)
	}

	// 加载配置到结构体
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	var grpcServerConfig GrpcServerConfig
	if err := loader.Load(&appConfig, &loggerConfig, &grpcServerConfig); err != nil {
		panic(err)
	}

	// 使用配置...
}

// 示例 2: 指定配置路径
func ExampleWithConfigPath() {
	// 指定配置文件路径
	loader, err := NewConfigLoader("local", "/path/to/config")
	if err != nil {
		panic(err)
	}

	var appConfig AppConfig
	if err := loader.Load(&appConfig); err != nil {
		panic(err)
	}
}

// 示例 3: 使用全局便捷函数（向后兼容）
func ExampleGlobalFunctions() {
	// 初始化全局配置
	InitConfig("local")

	// 加载配置
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	LoadCustomConfig(&appConfig, &loggerConfig)

	// 使用配置...
}

// 示例 4: 环境变量优先级
func ExampleEnvPriority() {
	// 如果设置了 DANDELION_ENV 环境变量，会优先使用环境变量的值
	// os.Setenv("DANDELION_ENV", "production")
	loader, err := NewConfigLoader("local") // 即使传入 "local"，也会使用 "production"
	if err != nil {
		panic(err)
	}
	// 实际加载的是 production 环境的配置
}

// 示例 5: 获取配置信息
func ExampleGetConfigInfo() {
	loader, err := NewConfigLoader("local")
	if err != nil {
		panic(err)
	}

	// 获取配置信息
	env := loader.GetEnv()           // "local"
	path := loader.GetConfigPath()   // "/path/to/config"
	name := loader.GetConfigName()   // "configs_local"
	format := loader.GetConfigFormat() // "yaml"

	// 使用配置信息...
}

// 示例 6: 高级用法 - 直接使用 Viper
func ExampleAdvancedUsage() {
	loader, err := NewConfigLoader("local")
	if err != nil {
		panic(err)
	}

	// 获取底层 viper 实例
	v := loader.GetViper()

	// 使用 viper 的高级功能
	value := v.GetString("some.nested.key")
	// ...
}
*/

