# QuickGo Framework

QuickGo is a lightweight, modular Go framework for building microservices with integrated observability features.

## Features

- Structured logging with trace context propagation
- Distributed tracing with OpenTelemetry/Jaeger
- Service discovery with etcd
- API gateway (HTTP to gRPC proxy)
- Graceful shutdown

## Components

- **logger**: Structured logging library with JSON output
- **tracing**: OpenTelemetry integration for distributed tracing
- **example/framework**: Complete microservices example with auth service and API gateway

## Quick Start

1. Start etcd:
   ```
   docker run -d --name etcd -p 2379:2379 -p 2380:2380 quay.io/coreos/etcd:v3.5.13 etcd --advertise-client-urls=http://127.0.0.1:2379 --listen-client-urls=http://0.0.0.0:2379
   ```

2. Run the auth service:
   ```
   cd example/framework/auth-server
   make proto && make build && make run
   ```

3. Run the gateway (in a new terminal):
   ```
   cd example/framework/gateway
   make build && make run
   ```

4. Test the API:
   ```
   curl -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" -d '{"username":"admin","password":"admin123"}'
   ```

## Documentation

- [Logger Documentation](logger/README.md)
- [Tracing Documentation](tracing/README.md)
- [Framework Example](example/framework/README.md)