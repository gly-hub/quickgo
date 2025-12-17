.PHONY: build run clean

# 构建服务
build:
	@echo "Building gateway..."
	@go build -o bin/gateway ./cmd/gateway
	@echo "Gateway built successfully!"

# 运行服务
run: build
	@echo "Running gateway..."
	@./bin/gateway

# 清理构建产物
clean:
	@echo "Cleaning build files..."
	@rm -rf bin
	@echo "Clean completed!"

