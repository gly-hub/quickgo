package logger

// 标准日志字段名常量
// 用于确保日志字段命名的一致性
const (
	// 链路追踪相关
	FieldTraceID  = "trace_id"  // 链路追踪 ID
	FieldSpanID   = "span_id"   // Span ID
	FieldParentID = "parent_id" // 父 Span ID

	// 请求相关
	FieldRequestID     = "request_id"     // 请求 ID
	FieldMethod        = "method"         // HTTP 方法或 gRPC 方法
	FieldPath          = "path"           // 请求路径
	FieldStatusCode    = "status_code"    // 响应状态码
	FieldDuration      = "duration"       // 耗时（毫秒）
	FieldDurationMs    = "duration_ms"    // 耗时（毫秒）
	FieldDurationSec   = "duration_sec"   // 耗时（秒）
	FieldContentLength = "content_length" // 内容长度
	FieldUserAgent     = "user_agent"     // User Agent
	FieldClientIP      = "client_ip"      // 客户端 IP
	FieldRemoteAddr    = "remote_addr"    // 远程地址

	// gRPC 相关
	FieldGRPCCode    = "grpc_code"    // gRPC 状态码
	FieldGRPCService = "grpc_service" // gRPC 服务名
	FieldGRPCMethod  = "grpc_method"  // gRPC 方法名

	// 服务相关
	FieldService     = "service"      // 服务名称
	FieldVersion     = "version"      // 服务版本
	FieldEnvironment = "environment"  // 环境（local/develop/release/production）
	FieldHostname    = "hostname"     // 主机名
	FieldInstanceID  = "instance_id"  // 实例 ID
	FieldPodName     = "pod_name"     // Kubernetes Pod 名称
	FieldNamespace   = "namespace"    // Kubernetes 命名空间

	// 错误相关
	FieldError      = "error"       // 错误信息
	FieldErrorCode  = "error_code"  // 错误码
	FieldErrorType  = "error_type"  // 错误类型
	FieldStackTrace = "stack_trace" // 堆栈信息

	// 用户相关
	FieldUserID    = "user_id"    // 用户 ID
	FieldUsername  = "username"   // 用户名
	FieldSessionID = "session_id" // 会话 ID
	FieldTenantID  = "tenant_id"  // 租户 ID

	// 数据库相关
	FieldDBType     = "db_type"     // 数据库类型
	FieldDBName     = "db_name"     // 数据库名
	FieldDBTable    = "db_table"    // 表名
	FieldDBQuery    = "db_query"    // SQL 查询
	FieldDBRows     = "db_rows"     // 影响行数
	FieldDBDuration = "db_duration" // 数据库查询耗时

	// Redis 相关
	FieldRedisCmd  = "redis_cmd"  // Redis 命令
	FieldRedisKey  = "redis_key"  // Redis Key
	FieldRedisAddr = "redis_addr" // Redis 地址

	// 消息队列相关
	FieldMQTopic     = "mq_topic"     // 消息主题
	FieldMQPartition = "mq_partition" // 分区
	FieldMQOffset    = "mq_offset"    // 偏移量
	FieldMQGroupID   = "mq_group_id"  // 消费组 ID

	// 通用业务字段
	FieldAction   = "action"    // 操作动作
	FieldResource = "resource"  // 资源类型
	FieldModule   = "module"    // 模块名
	FieldComponent = "component" // 组件名
)

// Fields 日志字段类型
type Fields map[string]interface{}

// WithRequest 添加请求相关字段
func (f Fields) WithRequest(method, path string, statusCode int, durationMs float64) Fields {
	f[FieldMethod] = method
	f[FieldPath] = path
	f[FieldStatusCode] = statusCode
	f[FieldDurationMs] = durationMs
	return f
}

// WithGRPC 添加 gRPC 相关字段
func (f Fields) WithGRPC(service, method string, code string, durationMs float64) Fields {
	f[FieldGRPCService] = service
	f[FieldGRPCMethod] = method
	f[FieldGRPCCode] = code
	f[FieldDurationMs] = durationMs
	return f
}

// WithError 添加错误相关字段
func (f Fields) WithError(err error, errCode string, errType string) Fields {
	if err != nil {
		f[FieldError] = err.Error()
	}
	if errCode != "" {
		f[FieldErrorCode] = errCode
	}
	if errType != "" {
		f[FieldErrorType] = errType
	}
	return f
}

// WithUser 添加用户相关字段
func (f Fields) WithUser(userID, username string) Fields {
	if userID != "" {
		f[FieldUserID] = userID
	}
	if username != "" {
		f[FieldUsername] = username
	}
	return f
}

// WithDB 添加数据库相关字段
func (f Fields) WithDB(dbType, dbName, table, query string, rows int64, durationMs float64) Fields {
	f[FieldDBType] = dbType
	if dbName != "" {
		f[FieldDBName] = dbName
	}
	if table != "" {
		f[FieldDBTable] = table
	}
	if query != "" {
		f[FieldDBQuery] = query
	}
	f[FieldDBRows] = rows
	f[FieldDBDuration] = durationMs
	return f
}

// WithService 添加服务相关字段
func (f Fields) WithService(service, version, env string) Fields {
	if service != "" {
		f[FieldService] = service
	}
	if version != "" {
		f[FieldVersion] = version
	}
	if env != "" {
		f[FieldEnvironment] = env
	}
	return f
}

// NewFields 创建新的字段 map
func NewFields() Fields {
	return make(Fields)
}
