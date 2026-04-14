#!/bin/bash
# Hades 高性能代理内核部署脚本
# 项目地址: https://github.com/Qing060325/Hades

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 项目目录
PROJECT_DIR="/workspace/Hades"
BIN_PATH="$PROJECT_DIR/bin/hades"
CONFIG_PATH="$PROJECT_DIR/configs/config.yaml"
PID_FILE="/var/run/hades.pid"

# 打印带颜色的消息
info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 启动服务
start() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        warn "Hades 已在运行中 (PID: $(cat $PID_FILE))"
        return 1
    fi
    
    info "启动 Hades..."
    nohup $BIN_PATH -c $CONFIG_PATH > /tmp/hades.log 2>&1 &
    echo $! > $PID_FILE
    sleep 1
    
    if kill -0 $(cat $PID_FILE) 2>/dev/null; then
        info "Hades 启动成功 (PID: $(cat $PID_FILE))"
        info "混合端口: 7890 (HTTP + SOCKS5)"
        info "API 端口: 9090"
        info "DNS 端口: 1053"
    else
        error "Hades 启动失败，请检查日志: /tmp/hades.log"
        rm -f $PID_FILE
        return 1
    fi
}

# 停止服务
stop() {
    if [ ! -f "$PID_FILE" ]; then
        warn "Hades 未运行"
        return 0
    fi
    
    PID=$(cat $PID_FILE)
    if kill -0 $PID 2>/dev/null; then
        info "停止 Hades (PID: $PID)..."
        kill $PID
        sleep 2
        if kill -0 $PID 2>/dev/null; then
            warn "强制终止..."
            kill -9 $PID
        fi
        info "Hades 已停止"
    else
        warn "Hades 进程不存在"
    fi
    rm -f $PID_FILE
}

# 重启服务
restart() {
    stop
    sleep 1
    start
}

# 查看状态
status() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat $PID_FILE) 2>/dev/null; then
        info "Hades 运行中 (PID: $(cat $PID_FILE))"
        echo ""
        echo "服务端口:"
        echo "  - 混合端口: 7890 (HTTP + SOCKS5)"
        echo "  - API 端口: 9090"
        echo "  - DNS 端口: 1053"
        echo ""
        echo "日志文件: /tmp/hades.log"
    else
        warn "Hades 未运行"
        rm -f $PID_FILE 2>/dev/null
    fi
}

# 查看日志
logs() {
    if [ -f "/tmp/hades.log" ]; then
        tail -f /tmp/hades.log
    else
        warn "日志文件不存在"
    fi
}

# 构建项目
build() {
    info "构建 Hades..."
    cd $PROJECT_DIR
    make deps
    make build
    info "构建完成: $BIN_PATH"
}

# 帮助信息
help() {
    echo "Hades 高性能代理内核管理脚本"
    echo ""
    echo "用法: $0 {start|stop|restart|status|logs|build}"
    echo ""
    echo "命令:"
    echo "  start    启动服务"
    echo "  stop     停止服务"
    echo "  restart  重启服务"
    echo "  status   查看状态"
    echo "  logs     查看日志"
    echo "  build    构建项目"
}

# 主入口
case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) restart ;;
    status)  status ;;
    logs)    logs ;;
    build)   build ;;
    *)       help ;;
esac
