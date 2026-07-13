# QuickGo development helpers
# Usage: make test | make vet | make deps-up | make example-simple

GO            ?= go
GOFLAGS       ?=
PKG           ?= ./...
# Example packages need external services; default unit tests exclude example/
TEST_PKGS     ?= $(shell $(GO) list ./... | grep -v '/example/')
DOCKER_COMPOSE ?= docker compose

.PHONY: help tidy vet test test-all deps-up deps-down example-simple example-simple-stop ci-local

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

tidy: ## go mod tidy
	$(GO) mod tidy

vet: ## go vet (excluding example apps)
	$(GO) vet $(TEST_PKGS)

test: ## Run unit tests (exclude example/)
	$(GO) test $(GOFLAGS) $(TEST_PKGS)

test-all: ## Run all package tests including example (may need deps)
	$(GO) test $(GOFLAGS) $(PKG)

test-race: ## Unit tests with race detector
	$(GO) test $(GOFLAGS) -race $(TEST_PKGS)

deps-up: ## Start local infra (etcd, jaeger, redis, mysql)
	$(DOCKER_COMPOSE) up -d

deps-down: ## Stop local infra
	$(DOCKER_COMPOSE) down

deps-up-minimal: ## Start only etcd + jaeger (enough for discovery + tracing demos)
	$(DOCKER_COMPOSE) up -d etcd jaeger

example-simple: ## Build & run simple example (rpc + gateway)
	@cd example/simple && ./start.sh all

example-simple-stop: ## Stop simple example
	@cd example/simple && ./start.sh stop

example-simple-test: ## Hit simple example APIs
	@cd example/simple && ./test_api.sh

ci-local: tidy vet test ## Mimic CI locally
	@echo "ci-local OK"
