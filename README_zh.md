# QuickGo 框架

轻量、模块化的 Go 微服务框架：配置、生命周期、gRPC/HTTP、数据访问与可观测性统一管理。

**模块路径：** `github.com/gly-hub/quickgo`

## 能力地图

| 领域 | 包 / 入口 | 说明 |
|------|-----------|------|
| 框架生命周期 | 根包 `quickgo`（`framework.go`） | Option 装配，Init/Start/Stop，失败回滚 |
| 配置 | `config.go` | Viper，`configs_{env}`，环境变量 `DANDELION_ENV` |
| gRPC | 根包 `grpc_*.go`、`grpc/` | 静态/etcd 发现、连接池、健康检查 |
| HTTP（Fiber） | 根包 `http_server.go`、`http/` | CORS、恢复、日志、/metrics |
| 业务响应约定 | `grpcep/` | 统一响应与网关辅助 |
| 错误模型 | `gerr/` | 类型化错误、是否可重试 |
| 日志 | `logger/` | 结构化 JSON + Trace 字段 |
| 链路追踪 | `tracing/` | OpenTelemetry，推荐 OTLP |
| 指标 | `metrics/` | Prometheus |
| 弹性 | `resilience/` | 熔断 / 限流 / 重试（暂需手动挂载） |
| 校验 | `validation/` | 结构体/标签校验 |
| 数据访问 | `db/gorm`、`db/mongodb`、`db/redis` | 多实例管理器 |

> 业务代码优先使用**根包 `quickgo`**（配置友好）；子包为底层实现。

## 功能特性

- 结构化日志与链路上下文传播
- 分布式追踪（OpenTelemetry / OTLP）
- Prometheus 指标
- gRPC 服务发现（静态或 etcd）
- HTTP ↔ gRPC 网关模式
- GORM / MongoDB / Redis 多客户端
- 优雅关闭与初始化回滚
- 熔断、限流、重试原语

## 环境要求

- Go **1.25.4+**（见 `go.mod` 的 `toolchain`）
- 可选 Docker 依赖：etcd、Jaeger、Redis、MySQL

## 快速开始（simple 示例，无需 etcd）

```bash
make test

# 可选：链路/发现相关依赖
make deps-up-minimal

cd example/simple
./start.sh all
./test_api.sh
./start.sh stop
```

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/api/v1/users/
```

## 最小用法

```go
package main

import "github.com/gly-hub/quickgo"

func main() {
	quickgo.InitConfig("local", "./config")

	var cfg struct {
		App        quickgo.AppConfig        `yaml:"app"`
		Logger     quickgo.LoggerConfig     `yaml:"logger"`
		HTTPServer quickgo.HTTPServerConfig `yaml:"httpServer"`
	}
	quickgo.LoadCustomConfig(&cfg)

	app, err := quickgo.NewFramework(
		quickgo.ConfigOptionWithApp(cfg.App),
		quickgo.ConfigOptionWithLogger(cfg.Logger),
		quickgo.ConfigOptionWithHTTPServer(&cfg.HTTPServer),
	)
	if err != nil {
		panic(err)
	}
	if err := app.Init(); err != nil {
		panic(err)
	}
	if err := app.Start(); err != nil {
		panic(err)
	}
	app.Wait() // 阻塞直到信号，然后优雅 Stop
}
```

| 环境变量 | 含义 |
|----------|------|
| `DANDELION_ENV` | 覆盖配置环境：`local` / `develop` / `release` / `production` |
| 配置文件 | `config/configs_{env}.yaml`（亦支持 json/toml/ini） |

## 完整示例（auth + gateway + etcd）

```bash
make deps-up
# 按 docker-compose 中的 MySQL 账号调整 example YAML 后：
cd example/framework/auth-server && make proto && make build && make run
cd example/framework/gateway && make build && make run
```

详见 [example/framework/QUICKSTART.md](example/framework/QUICKSTART.md)。

## 文档索引

| 文档 | 说明 |
|------|------|
| [docs/PR_PLAN.md](docs/PR_PLAN.md) | PR 拆分与路线图 |
| [docs/architecture.md](docs/architecture.md) | 架构与启动顺序 |
| [docs/config-reference.md](docs/config-reference.md) | 配置字段参考 |
| [logger/README.md](logger/README.md) | 日志库 |
| [tracing/README.md](tracing/README.md) | 链路追踪 |
| [example/simple/README.md](example/simple/README.md) | 静态发现示例 |
| [example/framework/QUICKSTART.md](example/framework/QUICKSTART.md) | 完整微服务示例 |

英文入口：[README.md](README.md)

## 开发命令

```bash
make help
make ci-local      # tidy + vet + test
make deps-up
make deps-down
```

CI：[.github/workflows/ci.yml](.github/workflows/ci.yml)
