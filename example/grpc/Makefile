.PHONY: proto build-server build-client run-server run-client clean

# Proto 文件路径
PROTO_DIR := proto
PROTO_FILE := $(PROTO_DIR)/hello.proto
GEN_DIR := $(PROTO_DIR)/gen

# 生成 Go 代码
proto:
	@echo "Generating Go code from proto..."
	@mkdir -p $(GEN_DIR)/proto
	@protoc --go_out=$(GEN_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_FILE)
	@echo "Proto code generated successfully!"

# 构建 server
build-server: proto
	@echo "Building server..."
	@go build -o bin/server ./server
	@echo "Server built successfully!"

# 构建 client
build-client: proto
	@echo "Building client..."
	@go build -o bin/client ./client
	@echo "Client built successfully!"

# 运行 server
run-server: build-server
	@echo "Running server..."
	@./bin/server

# 运行 client
run-client: build-client
	@echo "Running client..."
	@./bin/client

# 清理生成的文件
clean:
	@echo "Cleaning generated files..."
	@rm -rf $(GEN_DIR)
	@rm -rf bin
	@echo "Clean completed!"

