# QuickGo Framework

Lightweight, modular Go framework for microservices — configuration, lifecycle, gRPC/HTTP, data access, and observability in one place.

**Module:** `github.com/team-dandelion/quickgo`

## Capability map

| Area | Package / entry | Notes |
|------|-----------------|--------|
| Framework lifecycle | root `quickgo` (`framework.go`) | `NewFramework` + Option pattern, Init/Start/Stop rollback |
| Config | root `config.go` | Viper, `configs_{env}`, env `DANDELION_ENV` |
| gRPC server / client | root `grpc_*.go`, `grpc/` | etcd discovery, pool, health check |
| HTTP (Fiber) | root `http_server.go`, `http/` | CORS, recovery, logging, metrics endpoint |
| Business response | `grpcep/` | Common response + gateway helpers |
| Errors | `gerr/` | Typed errors, retryability |
| Logging | `logger/` | Structured JSON + trace fields |
| Tracing | `tracing/` | OpenTelemetry, OTLP (recommended) / Jaeger legacy |
| Metrics | `metrics/` | Prometheus HTTP/gRPC/pool/resilience |
| Resilience | `resilience/` | Circuit breaker, rate limit, retry (wire manually for now) |
| Validation | `validation/` | Struct/tag validation |
| Data | `db/gorm`, `db/mongodb`, `db/redis` | Multi-instance managers |

> Application code should prefer the **root `quickgo` package** (YAML-friendly configs). Lower-level packages are implementation details.

## Features

- Structured logging with trace context
- Distributed tracing (OpenTelemetry / OTLP)
- Prometheus metrics
- gRPC service discovery (static or etcd)
- HTTP ↔ gRPC gateway patterns
- GORM / MongoDB / Redis multi-client managers
- Graceful shutdown and init rollback
- Resilience primitives (circuit breaker, rate limit, retry)

## Requirements

- Go **1.25.4+** (see `go.mod` / `toolchain`)
- Optional local deps via Docker: etcd, Jaeger, Redis, MySQL

## Quick start (simple example, no etcd)

```bash
# unit tests
make test

# optional: infra for advanced demos
make deps-up-minimal   # etcd + jaeger

# simple static-discovery demo
cd example/simple
./start.sh all
./test_api.sh
./start.sh stop
```

Try the gateway:

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/api/v1/users/
```

## Minimal framework usage

```go
package main

import (
	"github.com/team-dandelion/quickgo"
)

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
	// register routes on app.HTTPServer(), then:
	if err := app.Start(); err != nil {
		panic(err)
	}
	app.Wait() // blocks until SIGINT/SIGTERM, then graceful Stop
}
```

Environment:

| Variable | Purpose |
|----------|---------|
| `DANDELION_ENV` | Overrides config env: `local`, `develop`, `release`, `production` |
| Config file | `config/configs_{env}.yaml` (json/toml/ini also supported) |

## Full stack example (auth + gateway + etcd)

```bash
make deps-up   # etcd, jaeger, redis, mysql

# adjust example YAML passwords to match docker-compose (mysql root/quickgo)
# then:
cd example/framework/auth-server && make proto && make build && make run
# another terminal
cd example/framework/gateway && make build && make run
```

See [example/framework/QUICKSTART.md](example/framework/QUICKSTART.md).

## Documentation

| Doc | Description |
|-----|-------------|
| [docs/PR_PLAN.md](docs/PR_PLAN.md) | Split improvement PRs and roadmap |
| [docs/architecture.md](docs/architecture.md) | Components, lifecycle, request path |
| [docs/config-reference.md](docs/config-reference.md) | YAML field reference |
| [logger/README.md](logger/README.md) | Logger API |
| [tracing/README.md](tracing/README.md) | Tracing / OTLP |
| [example/simple/README.md](example/simple/README.md) | Static discovery demo |
| [example/framework/QUICKSTART.md](example/framework/QUICKSTART.md) | etcd + auth + gateway |

Chinese overview: [README_zh.md](README_zh.md)

## Development

```bash
make help          # list targets
make ci-local      # tidy + vet + test
make deps-up       # docker compose infra
make deps-down
```

CI runs on push/PR via [.github/workflows/ci.yml](.github/workflows/ci.yml) (unit packages, excludes `example/`).

## License

See repository license (if published). Internal team module: `team-dandelion/quickgo`.
