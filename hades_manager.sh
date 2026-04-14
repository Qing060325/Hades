#!/bin/bash

# Hades 一键安装与管理脚本 (Enhanced Version)
# -------------------------------------------------------------------
# 支持二进制下载、源码构建、Systemd 服务管理、配置初始化及交互式菜单。
# -------------------------------------------------------------------

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# 变量定义
REPO="Qing060325/Hades"
GITHUB_API="https://api.github.com/repos/${REPO}"
GITHUB_RAW="https://raw.githubusercontent.com/${REPO}/main"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/hades"
LOG_DIR="/var/log/hades"
SERVICE_FILE="/etc/systemd/system/hades.service"

# -------------------------------------------------------------------
# 辅助函数
# -------------------------------------------------------------------

print_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

check_root() {
    if [ "$EUID" -ne 0 ]; then
        print_error "请使用 root 权限运行此脚本 (sudo)。"
    fi
}

detect_os_arch() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) print_error "不支持的架构: $ARCH" ;;
    esac
    print_info "检测到系统环境: $OS/$ARCH"
}

# -------------------------------------------------------------------
# 主要功能
# -------------------------------------------------------------------

# 安装 Hades
install_hades() {
    check_root
    detect_os_arch

    print_info "获取最新版本信息..."
    LATEST_TAG=$(curl -s "${GITHUB_API}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$LATEST_TAG" ]; then
        print_warning "无法获取最新版本，尝试使用默认版本 v0.2.0"
        LATEST_TAG="v0.2.0"
    fi

    BINARY_NAME="hades-${OS}-${ARCH}"
    DOWNLOAD_URL="${GITHUB_API}/releases/download/${LATEST_TAG}/${BINARY_NAME}"

    print_info "正在下载 Hades ${LATEST_TAG}..."
    if ! curl -L -o "/tmp/hades" "$DOWNLOAD_URL"; then
        print_warning "二进制文件下载失败，尝试从源码构建..."
        build_from_source
    else
        chmod +x /tmp/hades
        mv /tmp/hades "${INSTALL_DIR}/hades"
        print_success "二进制文件已安装到 ${INSTALL_DIR}/hades"
    fi

    setup_config
    setup_service
    print_success "Hades 安装完成！"
    display_status
}

# 从源码构建
build_from_source() {
    if ! command -v go &> /dev/null; then
        print_error "未检测到 Go 环境，无法从源码构建。请先安装 Go。"
    fi
    print_info "正在从源码构建 Hades..."
    TMP_DIR=$(mktemp -d)
    git clone "https://github.com/${REPO}.git" "$TMP_DIR"
    cd "$TMP_DIR"
    make deps || true
    go build -o bin/hades ./cmd/hades
    mv bin/hades "${INSTALL_DIR}/hades"
    chmod +x "${INSTALL_DIR}/hades"
    rm -rf "$TMP_DIR"
    print_success "源码构建并安装成功。"
}

# 初始化配置
setup_config() {
    print_info "初始化配置目录..."
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$LOG_DIR"
    if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
        print_info "下载默认配置文件..."
        curl -L -o "${CONFIG_DIR}/config.yaml" "${GITHUB_RAW}/configs/config.yaml" || \
        echo "mixed-port: 7890" > "${CONFIG_DIR}/config.yaml"
        print_success "配置文件已创建: ${CONFIG_DIR}/config.yaml"
    fi
}

# 设置 Systemd 服务
setup_service() {
    print_info "配置 Systemd 服务..."

    # 创建专用 hades 用户（如不存在）
    if ! id hades &>/dev/null; then
        print_info "创建 hades 用户..."
        useradd --system --no-create-home --shell /usr/sbin/nologin hades
    fi

    # 确保目录权限正确
    mkdir -p "${CONFIG_DIR}"
    mkdir -p "${LOG_DIR}"
    chown -R hades:hades "${CONFIG_DIR}" "${LOG_DIR}"

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=Hades Proxy Kernel
After=network.target

[Service]
Type=simple
User=hades
Group=hades
ExecStart=${INSTALL_DIR}/hades -c ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable hades
    print_success "Systemd 服务已注册并启用（以 hades 用户运行）。"
}

# 管理功能
start_hades() { systemctl start hades && print_success "Hades 已启动。"; }
stop_hades() { systemctl stop hades && print_success "Hades 已停止。"; }
restart_hades() { systemctl restart hades && print_success "Hades 已重启。"; }
show_logs() { journalctl -u hades -f; }

display_status() {
    if systemctl is-active --quiet hades; then
        echo -e "状态: ${GREEN}运行中${NC}"
    else
        echo -e "状态: ${RED}未运行${NC}"
    fi
    echo -e "配置文件: ${BLUE}${CONFIG_DIR}/config.yaml${NC}"
    echo -e "二进制路径: ${BLUE}${INSTALL_DIR}/hades${NC}"
}

uninstall_hades() {
    check_root
    print_warning "确定要卸载 Hades 吗？(y/N)"
    read -r confirm
    if [[ "$confirm" =~ ^[yY]$ ]]; then
        systemctl stop hades || true
        systemctl disable hades || true
        rm -f "$SERVICE_FILE"
        systemctl daemon-reload
        rm -f "${INSTALL_DIR}/hades"
        print_success "Hades 已成功卸载。"
    fi
}

# -------------------------------------------------------------------
# 主菜单
# -------------------------------------------------------------------

show_menu() {
    echo -e "\n${BLUE}╔══════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║          Hades 内核管理工具          ║${NC}"
    echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
    echo -e "  ${GREEN}1)${NC} 安装/更新 Hades"
    echo -e "  ${GREEN}2)${NC} 启动服务"
    echo -e "  ${GREEN}3)${NC} 停止服务"
    echo -e "  ${GREEN}4)${NC} 重启服务"
    echo -e "  ${GREEN}5)${NC} 查看实时日志"
    echo -e "  ${GREEN}6)${NC} 卸载 Hades"
    echo -e "  ${GREEN}7)${NC} 查看运行状态"
    echo -e "  ${GREEN}0)${NC} 退出"
    echo -n "请输入选项 [0-7]: "
}

main() {
    while true; do
        show_menu
        read -r choice
        case "$choice" in
            1) install_hades ;;
            2) start_hades ;;
            3) stop_hades ;;
            4) restart_hades ;;
            5) show_logs ;;
            6) uninstall_hades ;;
            7) display_status ;;
            0) exit 0 ;;
            *) print_warning "无效选项。" ;;
        esac
    done
}

main
