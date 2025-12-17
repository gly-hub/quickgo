# HTTP 服务示例

这是一个完整的 HTTP 服务示例，演示了如何使用 QuickGo 的 HTTP 封装库（基于 Fiber）。

## 功能特性

- ✅ 使用 QuickGo 的 HTTP 封装库
- ✅ 集成 logger 库进行日志输出
- ✅ 链路追踪支持（trace ID）
- ✅ 请求 ID 支持
- ✅ CORS 支持
- ✅ 错误恢复（panic recovery）
- ✅ 请求日志记录
- ✅ 优雅关闭

## 目录结构

```
example/http/
├── main.go      # 主程序
└── README.md    # 本文件
```

## 使用方法

### 1. 运行服务

```bash
cd example/http
go run main.go
```

服务将在 `http://localhost:9999` 启动。

### 2. 测试接口

#### 首页
```bash
curl http://localhost:9999/
```

#### 健康检查
```bash
curl http://localhost:9999/health
```

#### 获取用户列表
```bash
curl http://localhost:9999/api/users
```

#### 获取单个用户
```bash
curl http://localhost:9999/api/users/1
```

#### 创建用户
```bash
curl -X POST http://localhost:9999/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"David","email":"david@example.com"}'
```

#### 链路追踪示例
```bash
curl http://localhost:9999/api/trace
```

#### 带 trace ID 的请求（链路追踪）
```bash
curl -H "X-Trace-ID: my-custom-trace-id" http://localhost:9999/api/trace
```

## API 端点

### GET /
首页，返回欢迎信息

**响应示例：**
```json
{
  "message": "Welcome to QuickGo HTTP Server",
  "trace_id": "a1b2c3d4e5f6g7h8",
  "request_id": "i9j0k1l2m3n4o5p6",
  "timestamp": "2025-12-12T12:00:00Z"
}
```

### GET /health
健康检查端点

**响应示例：**
```json
{
  "status": "ok",
  "service": "quickgo-http",
  "trace_id": "a1b2c3d4e5f6g7h8"
}
```

### GET /api/users
获取用户列表

**响应示例：**
```json
{
  "users": [
    {
      "id": 1,
      "name": "Alice",
      "email": "alice@example.com"
    },
    {
      "id": 2,
      "name": "Bob",
      "email": "bob@example.com"
    }
  ],
  "count": 2,
  "trace_id": "a1b2c3d4e5f6g7h8"
}
```

### GET /api/users/:id
获取单个用户

**路径参数：**
- `id`: 用户 ID

**响应示例：**
```json
{
  "user": {
    "id": "1",
    "name": "User 1",
    "email": "user1@example.com"
  },
  "trace_id": "a1b2c3d4e5f6g7h8"
}
```

### POST /api/users
创建用户

**请求体：**
```json
{
  "name": "David",
  "email": "david@example.com"
}
```

**响应示例：**
```json
{
  "message": "User created successfully",
  "user": {
    "id": 4,
    "name": "David",
    "email": "david@example.com"
  },
  "trace_id": "a1b2c3d4e5f6g7h8"
}
```

### GET /api/trace
链路追踪信息展示

**响应示例：**
```json
{
  "trace_id": "a1b2c3d4e5f6g7h8",
  "span_id": "q1r2s3t4",
  "request_id": "i9j0k1l2m3n4o5p6",
  "message": "This endpoint demonstrates trace ID propagation",
  "headers": {
    "x-trace-id": "a1b2c3d4e5f6g7h8",
    "x-request-id": "i9j0k1l2m3n4o5p6"
  }
}
```

## 链路追踪

服务支持链路追踪功能：

1. **自动生成 trace ID**：如果请求头中没有 `X-Trace-ID`，服务会自动生成一个
2. **传递 trace ID**：trace ID 会通过响应头 `X-Trace-ID` 返回给客户端
3. **日志关联**：所有日志都会包含 trace ID，方便追踪请求链路
4. **统一标识**：`trace_id` 和 `request_id` 使用同一个值，统一使用 `X-Trace-ID` 请求头，避免混淆

### 使用方式

#### 方式 1：自动生成
```bash
curl http://localhost:9999/api/trace
```
服务会自动生成 trace ID。

#### 方式 2：客户端传递
```bash
curl -H "X-Trace-ID: my-custom-trace-id" http://localhost:9999/api/trace
```
服务会使用客户端传递的 trace ID。

**注意**：统一使用 `X-Trace-ID` 请求头，`request_id` 和 `trace_id` 使用同一个值，避免后续排查问题时需要区分使用哪个请求头。

## 日志输出

服务会输出详细的日志信息，包括：
- 请求方法、路径、IP、User-Agent
- 响应状态码、耗时
- 链路追踪信息（trace ID）
- 错误信息（如果有）

**日志示例：**
```
2025-12-12 12:00:00 [INFO] HTTP request: method=GET, path=/api/users, ip=127.0.0.1, user_agent=curl/7.68.0 [trace_id:a1b2c3d4e5f6g7h8]
2025-12-12 12:00:00 [INFO] HTTP request success: method=GET, path=/api/users, status=200, duration=1.234ms [trace_id:a1b2c3d4e5f6g7h8]
```

## 优雅关闭

服务支持优雅关闭：
- 按 `Ctrl+C` 或发送 `SIGTERM` 信号
- 服务会等待当前请求处理完成后关闭

## 扩展

你可以基于这个示例扩展：

1. **添加更多路由**
   - 在 `setupRoutes` 函数中添加新的路由

2. **添加数据库连接**
   - 集成数据库（如 MySQL、PostgreSQL、MongoDB）

3. **添加认证中间件**
   - 实现 JWT 认证或其他认证方式

4. **添加限流中间件**
   - 使用专业的限流库实现请求限流

5. **添加缓存**
   - 使用 Redis 或其他缓存系统

