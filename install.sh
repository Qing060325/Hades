#!/bin/bash
#
# Hades 一键安装脚本
# 高性能代理内核 - 开箱即用
#
# 用法: curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | bash
#

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# 版本号
VERSION="v0.2.0"
REPO="Qing060325/Hades"
GITHUB_API="https://api.github.com/repos/${REPO}"
GITHUB_RAW="https://raw.githubusercontent.com/${REPO}"

# 安装目录
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/hades"
LOG_DIR="/var/log/hades"

# 打印函数
info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }
step() { echo -e "${BLUE}==>${NC} ${BOLD}$1${NC}"; }

# 检测操作系统
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux" ;;
        Darwin*)    echo "darwin" ;;
        CYGWIN*)    echo "windows" ;;
        MINGW*)     echo "windows" ;;
        *)          echo "unknown" ;;
    esac
}

# 检测架构
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        armv7l|armhf)   echo "arm" ;;
        i386|i686)      echo "386" ;;
        *)              echo "unknown" ;;
    esac
}

# 检测是否有 root 权限
check_root() {
    if [ "$EUID" -ne 0 ] && [ "$1" != "--user" ]; then
        warn "建议使用 root 权限安装，或使用 --user 参数安装到用户目录"
        INSTALL_DIR="$HOME/.local/bin"
        CONFIG_DIR="$HOME/.config/hades"
        LOG_DIR="$HOME/.local/var/log/hades"
    fi
}

# 获取最新版本
get_latest_version() {
    if command -v curl &> /dev/null; then
        curl -fsSL "${GITHUB_API}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget &> /dev/null; then
        wget -qO- "${GITHUB_API}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        echo "$VERSION"
    fi
}

# 下载文件
download_file() {
    local url="$1"
    local output="$2"

    if command -v curl &> /dev/null; then
        curl -fsSL "$url" -o "$output"
    elif command -v wget &> /dev/null; then
        wget -q "$url" -O "$output"
    else
        error "需要 curl 或 wget 来下载文件"
        exit 1
    fi
}

# 安装二进制文件
install_binary() {
    local os="$1"
    local arch="$2"
    local version="$3"

    step "下载 Hades ${version} (${os}/${arch})..."

    local binary_name="hades-${os}-${arch}"
    local download_url="${GITHUB_API}/releases/download/${version}/${binary_name}"

    if [ "$os" = "windows" ]; then
        binary_name="${binary_name}.exe"
        download_url="${GITHUB_API}/releases/download/${version}/${binary_name}"
    fi

    local tmp_file="/tmp/${binary_name}"

    # 尝试下载预编译二进制
    if download_file "$download_url" "$tmp_file" 2>/dev/null; then
        info "下载成功"
    else
        warn "预编译二进制不可用，将本地构建..."
        build_from_source
        return
    fi

    # 安装
    chmod +x "$tmp_file"
    mv "$tmp_file" "${INSTALL_DIR}/hades"

    info "二进制文件已安装到 ${INSTALL_DIR}/hades"
}

# 从源码构建
build_from_source() {
    step "从源码构建..."

    # 检查 Go 环境
    if ! command -v go &> /dev/null; then
        error "需要 Go 环境来构建"
        exit 1
    fi

    # 克隆仓库
    local tmp_dir="/tmp/hades-build"
    rm -rf "$tmp_dir"
    git clone "https://github.com/${REPO}.git" "$tmp_dir"
    cd "$tmp_dir"

    # 构建
    make deps
    make build

    # 安装
    mv bin/hades "${INSTALL_DIR}/hades"
    chmod +x "${INSTALL_DIR}/hades"

    # 清理
    rm -rf "$tmp_dir"

    info "构建完成"
}

# 创建配置目录和默认配置
setup_config() {
    step "创建配置..."

    mkdir -p "$CONFIG_DIR"
    mkdir -p "$LOG_DIR"

    # 下载默认配置
    if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
        download_file "${GITHUB_RAW}/main/configs/config.yaml" "${CONFIG_DIR}/config.yaml"
        info "默认配置已创建: ${CONFIG_DIR}/config.yaml"
    else
        warn "配置文件已存在，跳过"
    fi

    # 创建服务管理脚本
    cat > "${INSTALL_DIR}/hades-ctl" << 'SCRIPT'
#!/bin/bash
# Hades 服务管理脚本

CONFIG_FILE="/etc/hades/config.yaml"
PID_FILE="/var/run/hades.pid"
LOG_FILE="/var/log/hades/hades.log"

start() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Hades 已在运行"
        return 1
    fi
    nohup hades -c "$CONFIG_FILE" > "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    echo "Hades 已启动 (PID: $(cat $PID_FILE))"
}

stop() {
    if [ -f "$PID_FILE" ]; then
        kill $(cat "$PID_FILE") 2>/dev/null
        rm -f "$PID_FILE"
        echo "Hades 已停止"
    else
        echo "Hades 未运行"
    fi
}

restart() {
    stop
    sleep 1
    start
}

status() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Hades 运行中 (PID: $(cat $PID_FILE))"
    else
        echo "Hades 未运行"
        rm -f "$PID_FILE" 2>/dev/null
    fi
}

logs() {
    tail -f "$LOG_FILE"
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) restart ;;
    status)  status ;;
    logs)    logs ;;
    *)       echo "用法: hades-ctl {start|stop|restart|status|logs}" ;;
esac
SCRIPT
    chmod +x "${INSTALL_DIR}/hades-ctl"

    info "管理脚本已安装: hades-ctl"
}

# 安装系统服务
install_service() {
    local os="$1"

    step "安装系统服务..."

    if [ "$os" = "linux" ]; then
        # systemd
        if command -v systemctl &> /dev/null; then
            cat > /etc/systemd/system/hades.service << 'SERVICE'
[Unit]
Description=Hades Proxy Kernel
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hades -c /etc/hades/config.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
SERVICE
            systemctl daemon-reload
            systemctl enable hades
            info "systemd 服务已安装: systemctl {start|stop|status} hades"
            return
        fi
    fi

    if [ "$os" = "darwin" ]; then
        # launchd
        local plist_path="$HOME/Library/LaunchAgents/com.hades.plist"
        cat > "$plist_path" << SERVICE
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.hades</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/hades</string>
        <string>-c</string>
        <string>${CONFIG_DIR}/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
SERVICE
        launchctl load "$plist_path" 2>/dev/null || true
        info "launchd 服务已安装"
        return
    fi

    warn "未检测到支持的 init 系统，请手动启动"
}

# 配置向导
config_wizard() {
    step "配置向导..."

    echo ""
    echo "请选择配置模式:"
    echo "  1) 快速配置 (仅设置端口)"
    echo "  2) 完整配置 (交互式)"
    echo "  3) 跳过 (使用默认配置)"
    echo ""
    read -p "请选择 [1-3]: " choice

    case "$choice" in
        1)
            read -p "混合端口 [7890]: " port
            port=${port:-7890}
            sed -i.bak "s/mixed-port: .*/mixed-port: ${port}/" "${CONFIG_DIR}/config.yaml"
            rm -f "${CONFIG_DIR}/config.yaml.bak"
            info "混合端口已设置为 ${port}"
            ;;
        2)
            echo "完整配置向导开发中..."
            ;;
        3)
            info "使用默认配置"
            ;;
        *)
            info "使用默认配置"
            ;;
    esac
}

# 显示完成信息
show_complete() {
    echo ""
    echo -e "${GREEN}${BOLD}========================================${NC}"
    echo -e "${GREEN}${BOLD}  Hades 安装完成！${NC}"
    echo -e "${GREEN}${BOLD}========================================${NC}"
    echo ""
    echo "版本: $(hades -v)"
    echo ""
    echo "快速开始:"
    echo "  hades -c ${CONFIG_DIR}/config.yaml    # 启动"
    echo "  hades-ctl start                       # 服务方式启动"
    echo "  hades-ctl status                      # 查看状态"
    echo ""
    echo "配置文件: ${CONFIG_DIR}/config.yaml"
    echo "日志文件: ${LOG_DIR}/hades.log"
    echo ""
    echo "文档: https://github.com/${REPO}#readme"
    echo ""
}

# 卸载
uninstall() {
    step "卸载 Hades..."

    # 停止服务
    if command -v systemctl &> /dev/null; then
        systemctl stop hades 2>/dev/null || true
        systemctl disable hades 2>/dev/null || true
        rm -f /etc/systemd/system/hades.service
        systemctl daemon-reload
    fi

    # 删除文件
    rm -f "${INSTALL_DIR}/hades"
    rm -f "${INSTALL_DIR}/hades-ctl"
    rm -rf "$CONFIG_DIR"
    rm -rf "$LOG_DIR"
    rm -f "/var/run/hades.pid"

    info "Hades 已卸载"
}

# 主函数
main() {
    echo -e "${BOLD}"
    echo "  _    _           _     "
    echo " | |  | |         | |    "
    echo " | |__| | __ _ ___| |__  "
    echo " |  __  |/ _\` / __| '_ \ "
    echo " | |  | | (_| \__ \ | | |"
    echo " |_|  |_|\__,_|___/_| |_|"
    echo ""
    echo "  高性能代理内核 - 开箱即用"
    echo -e "${NC}"
    echo ""

    # 解析参数
    case "${1:-}" in
        uninstall)
            uninstall
            exit 0
            ;;
        --user)
            INSTALL_DIR="$HOME/.local/bin"
            CONFIG_DIR="$HOME/.config/hades"
            LOG_DIR="$HOME/.local/var/log/hades"
            ;;
    esac

    # 检测环境
    local os=$(detect_os)
    local arch=$(detect_arch)

    info "检测到系统: ${os}/${arch}"

    if [ "$os" = "unknown" ] || [ "$arch" = "unknown" ]; then
        error "不支持的系统或架构"
        exit 1
    fi

    # 检查权限
    check_root "$@"

    # 获取版本
    local version=$(get_latest_version)
    info "最新版本: ${version}"

    # 安装
    install_binary "$os" "$arch" "$version"
    setup_config

    # 询问是否安装系统服务
    echo ""
    read -p "是否安装系统服务? [Y/n]: " install_svc
    case "$install_svc" in
        n|N)
            warn "跳过系统服务安装"
            ;;
        *)
            install_service "$os"
            ;;
    esac

    # 配置向导
    echo ""
    read -p "是否运行配置向导? [y/N]: " run_wizard
    case "$run_wizard" in
        y|Y)
            config_wizard
            ;;
    esac

    # 完成
    show_complete
}

# 运行
main "$@"
