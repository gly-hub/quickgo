.PHONY: proto build run clean

# Proto 文件路径
PROTO_DIR := api/proto
PROTO_FILE := $(PROTO_DIR)/auth.proto
GEN_DIR := $(PROTO_DIR)/gen

# 生成 Go 代码
proto:
	@echo "Generating Go code from proto..."
	@mkdir -p $(GEN_DIR)
	@protoc -I../../../ -I$(PROTO_DIR) \
		--go_out=$(GEN_DIR) \
		--go_opt=paths=source_relative \
		--go_opt=Mapi/proto/auth.proto=quickgo/example/framework/auth-server/api/proto/gen \
		--go-grpc_out=$(GEN_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_FILE)
	@echo "Proto code generated successfully!"

# 构建服务
build: proto
	@echo "Building auth-server..."
	@go build -o bin/auth-server ./cmd/server
	@echo "Auth-server built successfully!"

# 运行服务
run: build
	@echo "Running auth-server..."
	@./bin/auth-server

# 清理生成的文件
clean:
	@echo "Cleaning generated files..."
	@rm -rf $(GEN_DIR)
	@rm -rf bin
	@echo "Clean completed!"

