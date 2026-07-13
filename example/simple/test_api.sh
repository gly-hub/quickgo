#!/bin/bash

# QuickGo 简单示例 - API 测试脚本
# 使用方法:
#   1. 启动 RPC 服务: cd rpc-server/cmd && go run main.go
#   2. 启动网关服务: cd gateway/cmd && go run main.go
#   3. 运行测试: ./test_api.sh

set -e

# 配置
GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:8080}"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 辅助函数
print_header() {
    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}$1${NC}"
    echo -e "${YELLOW}========================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "  $1"
}

test_api() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4

    echo ""
    echo -e "${YELLOW}▶ $description${NC}"
    echo "  $method $endpoint"

    if [ "$method" == "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$GATEWAY_URL$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$GATEWAY_URL$endpoint")
    fi

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    echo "  HTTP Status: $http_code"
    echo "  Response:"
    echo "$body" | jq . 2>/dev/null || echo "$body"

    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_success "请求成功"
    else
        print_error "请求失败"
    fi
}

# 检查依赖
check_dependencies() {
    if ! command -v curl &> /dev/null; then
        echo "错误: 需要安装 curl"
        exit 1
    fi
    if ! command -v jq &> /dev/null; then
        echo "警告: 未安装 jq，输出将不会格式化"
    fi
}

# 等待服务就绪
wait_for_service() {
    local url=$1
    local max_attempts=30
    local attempt=0

    echo "等待服务就绪: $url"
    while [ $attempt -lt $max_attempts ]; do
        if curl -s "$url/health" > /dev/null 2>&1; then
            print_success "服务已就绪"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    print_error "服务未就绪，超时退出"
    exit 1
}

# 主测试流程
main() {
    check_dependencies

    print_header "QuickGo API 测试"
    echo "网关地址: $GATEWAY_URL"

    # 等待服务就绪
    wait_for_service "$GATEWAY_URL"

    print_header "1. 健康检查"
    test_api "GET" "/health" "" "检查网关和上游服务健康状态"

    print_header "2. 用户列表"
    test_api "GET" "/api/v1/users/" "" "获取用户列表 (默认分页)"
    test_api "GET" "/api/v1/users/?page=1&page_size=2" "" "获取用户列表 (分页参数)"

    print_header "3. 获取用户"
    test_api "GET" "/api/v1/users/1" "" "获取用户 ID=1 (admin)"
    test_api "GET" "/api/v1/users/2" "" "获取用户 ID=2 (test)"
    test_api "GET" "/api/v1/users/999" "" "获取不存在的用户 ID=999"

    print_header "4. 创建用户"
    test_api "POST" "/api/v1/users/" \
        '{"username":"newuser","email":"newuser@example.com","phone":"13900000001"}' \
        "创建新用户"

    test_api "POST" "/api/v1/users/" \
        '{"username":"","email":"invalid@example.com"}' \
        "创建用户 - 用户名为空 (应该失败)"

    test_api "POST" "/api/v1/users/" \
        '{"username":"admin","email":"admin2@example.com"}' \
        "创建用户 - 用户名已存在 (应该失败)"

    print_header "5. 验证创建的用户"
    test_api "GET" "/api/v1/users/" "" "再次获取用户列表，确认新用户已创建"

    print_header "测试完成"
    echo ""
    echo "所有测试已完成！"
    echo ""
}

# 运行
main "$@"
