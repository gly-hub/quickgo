package quickgo

// 使用 Option 模式的框架初始化示例

/*
// 示例 1: 只初始化 gRPC Server（认证服务场景）
func ExampleAuthServer() {
	// 初始化配置
	InitConfig("local")

	// 加载配置
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	var grpcServerConfig GrpcServerConfig
	LoadCustomConfig(&appConfig, &loggerConfig, &grpcServerConfig)

	// 创建框架，只初始化需要的组件
	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		ConfigOptionWithGrpcServer(&grpcServerConfig),
		// 不需要的组件直接不添加即可，例如：
		// ConfigOptionWithGrpcClient(&grpcClientConfig),  // 注释掉，不会初始化
		// ConfigOptionWithHTTPServer(&httpServerConfig),  // 注释掉，不会初始化
	)
	if err != nil {
		panic(err)
	}

	// 初始化并启动
	app.Init()
	app.Start()
	app.Wait()
}

// 示例 2: 初始化 HTTP Server 和 gRPC Client（网关服务场景）
func ExampleGateway() {
	// 初始化配置
	InitConfig("local")

	// 加载配置
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	var grpcClientConfig GrpcClientConfig
	var httpServerConfig HTTPServerConfig
	LoadCustomConfig(&appConfig, &loggerConfig, &grpcClientConfig, &httpServerConfig)

	// 创建框架，只初始化需要的组件
	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		ConfigOptionWithGrpcClient(&grpcClientConfig),
		ConfigOptionWithHTTPServer(&httpServerConfig),
		// 不需要 gRPC Server，直接不添加
	)
	if err != nil {
		panic(err)
	}

	// 初始化并启动
	app.Init()
	app.Start()
	app.Wait()
}

// 示例 3: 同时使用多个组件
func ExampleFullStack() {
	// 初始化配置
	InitConfig("local")

	// 加载配置
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	var grpcServerConfig GrpcServerConfig
	var grpcClientConfig GrpcClientConfig
	var httpServerConfig HTTPServerConfig
	LoadCustomConfig(&appConfig, &loggerConfig, &grpcServerConfig, &grpcClientConfig, &httpServerConfig)

	// 创建框架，初始化所有需要的组件
	app, err := NewFramework(
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
		ConfigOptionWithGrpcServer(&grpcServerConfig),
		ConfigOptionWithGrpcClient(&grpcClientConfig),
		ConfigOptionWithHTTPServer(&httpServerConfig),
	)
	if err != nil {
		panic(err)
	}

	// 初始化并启动
	app.Init()
	app.Start()
	app.Wait()
}

// 示例 4: 动态控制组件初始化（根据条件）
func ExampleConditional() {
	// 初始化配置
	InitConfig("local")

	// 加载配置
	var appConfig AppConfig
	var loggerConfig LoggerConfig
	var grpcServerConfig GrpcServerConfig
	var httpServerConfig HTTPServerConfig
	LoadCustomConfig(&appConfig, &loggerConfig, &grpcServerConfig, &httpServerConfig)

	// 根据环境或条件决定初始化哪些组件
	opts := []FrameworkOption{
		ConfigOptionWithApp(appConfig),
		ConfigOptionWithLogger(loggerConfig),
	}

	// 根据条件添加组件
	if needGrpcServer {
		opts = append(opts, ConfigOptionWithGrpcServer(&grpcServerConfig))
	}

	if needHTTPServer {
		opts = append(opts, ConfigOptionWithHTTPServer(&httpServerConfig))
	}

	// 创建框架
	app, err := NewFramework(opts...)
	if err != nil {
		panic(err)
	}

	// 初始化并启动
	app.Init()
	app.Start()
	app.Wait()
}
*/

