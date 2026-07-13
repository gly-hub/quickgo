#!/bin/bash

# QuickGo 简单示例 - 启动脚本
# 使用方法: ./start.sh [rpc|gateway|all|stop]

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RPC_PID_FILE="$SCRIPT_DIR/.rpc.pid"
GATEWAY_PID_FILE="$SCRIPT_DIR/.gateway.pid"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 启动 RPC 服务
start_rpc() {
    if [ -f "$RPC_PID_FILE" ]; then
        pid=$(cat "$RPC_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            print_warn "RPC 服务已在运行 (PID: $pid)"
            return
        fi
    fi

    print_info "启动 RPC 服务..."
    cd "$SCRIPT_DIR/rpc-server/cmd"
    nohup go run main.go > "$SCRIPT_DIR/.rpc.log" 2>&1 &
    echo $! > "$RPC_PID_FILE"
    cd "$SCRIPT_DIR"
    print_info "RPC 服务已启动 (PID: $(cat "$RPC_PID_FILE"))"
    print_info "RPC 服务地址: 127.0.0.1:9001"
    print_info "RPC 日志: $SCRIPT_DIR/.rpc.log"
}

# 启动网关服务
start_gateway() {
    if [ -f "$GATEWAY_PID_FILE" ]; then
        pid=$(cat "$GATEWAY_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            print_warn "网关服务已在运行 (PID: $pid)"
            return
        fi
    fi

    print_info "启动网关服务..."
    cd "$SCRIPT_DIR/gateway/cmd"
    nohup go run main.go > "$SCRIPT_DIR/.gateway.log" 2>&1 &
    echo $! > "$GATEWAY_PID_FILE"
    cd "$SCRIPT_DIR"
    print_info "网关服务已启动 (PID: $(cat "$GATEWAY_PID_FILE"))"
    print_info "网关服务地址: http://127.0.0.1:8080"
    print_info "网关日志: $SCRIPT_DIR/.gateway.log"
}

# 停止所有服务
stop_all() {
    print_info "停止所有服务..."

    if [ -f "$GATEWAY_PID_FILE" ]; then
        pid=$(cat "$GATEWAY_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            print_info "网关服务已停止 (PID: $pid)"
        fi
        rm -f "$GATEWAY_PID_FILE"
    fi

    if [ -f "$RPC_PID_FILE" ]; then
        pid=$(cat "$RPC_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            print_info "RPC 服务已停止 (PID: $pid)"
        fi
        rm -f "$RPC_PID_FILE"
    fi

    # 清理可能残留的进程
    pkill -f "go run main.go" 2>/dev/null || true

    print_info "所有服务已停止"
}

# 显示状态
status() {
    echo ""
    echo "服务状态:"
    echo "=========="

    if [ -f "$RPC_PID_FILE" ]; then
        pid=$(cat "$RPC_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            echo -e "  RPC 服务:    ${GREEN}运行中${NC} (PID: $pid)"
        else
            echo -e "  RPC 服务:    ${RED}已停止${NC}"
        fi
    else
        echo -e "  RPC 服务:    ${RED}未启动${NC}"
    fi

    if [ -f "$GATEWAY_PID_FILE" ]; then
        pid=$(cat "$GATEWAY_PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            echo -e "  网关服务:   ${GREEN}运行中${NC} (PID: $pid)"
        else
            echo -e "  网关服务:   ${RED}已停止${NC}"
        fi
    else
        echo -e "  网关服务:   ${RED}未启动${NC}"
    fi

    echo ""
}

# 显示帮助
usage() {
    echo "QuickGo 简单示例 - 启动脚本"
    echo ""
    echo "使用方法: $0 [命令]"
    echo ""
    echo "命令:"
    echo "  rpc       启动 RPC 服务"
    echo "  gateway   启动网关服务"
    echo "  all       启动所有服务"
    echo "  stop      停止所有服务"
    echo "  status    查看服务状态"
    echo "  restart   重启所有服务"
    echo "  test      启动服务并运行测试"
    echo "  help      显示帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 all       # 启动所有服务"
    echo "  $0 test      # 启动服务并运行 API 测试"
    echo "  $0 stop      # 停止所有服务"
    echo ""
}

# 主逻辑
case "${1:-help}" in
    rpc)
        start_rpc
        ;;
    gateway)
        start_gateway
        ;;
    all)
        start_rpc
        sleep 2
        start_gateway
        sleep 1
        status
        echo "提示: 运行 ./test_api.sh 测试 API"
        ;;
    stop)
        stop_all
        ;;
    status)
        status
        ;;
    restart)
        stop_all
        sleep 1
        start_rpc
        sleep 2
        start_gateway
        sleep 1
        status
        ;;
    test)
        start_rpc
        sleep 2
        start_gateway
        sleep 2
        echo ""
        print_info "运行 API 测试..."
        "$SCRIPT_DIR/test_api.sh"
        ;;
    help|--help|-h)
        usage
        ;;
    *)
        print_error "未知命令: $1"
        usage
        exit 1
        ;;
esac
