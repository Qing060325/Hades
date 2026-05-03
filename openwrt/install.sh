#!/bin/sh
# Hades OpenWrt 一键安装脚本
# 用法: sh install.sh [arch]
# 支持架构: mips mipsle arm arm64 aarch64

set -e

REPO="Qing060325/Hades"
VERSION="v1.0.0"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

INSTALL_DIR="/usr/bin"
CONFIG_DIR="/etc/hades"
INIT_DIR="/etc/init.d"
UCI_DIR="/etc/config"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo "${RED}[ERROR]${NC} $1"; }

# 检测架构
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)   echo "amd64" ;;
        i386|i686)      echo "386" ;;
        aarch64|arm64)  echo "arm64" ;;
        armv7*|armhf)   echo "armv7" ;;
        armv6*)         echo "armv6" ;;
        mips64le)       echo "mips64le" ;;
        mips64)         echo "mips64" ;;
        mipsel)         echo "mipsle" ;;
        mips)           echo "mips" ;;
        riscv64)        echo "riscv64" ;;
        s390x)          echo "s390x" ;;
        *)
            log_error "不支持的架构: $arch"
            exit 1
            ;;
    esac
}

# 检测系统
detect_os() {
    if [ -f /etc/openwrt_release ]; then
        echo "openwrt"
    elif [ -f /etc/os-release ]; then
        . /etc/os-release
        echo "$ID"
    else
        echo "unknown"
    fi
}

# 检查 root
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        log_error "需要 root 权限运行"
        exit 1
    fi
}

# 检查依赖
check_deps() {
    for cmd in curl wget; do
        if command -v "$cmd" >/dev/null 2>&1; then
            echo "$cmd"
            return 0
        fi
    done
    log_error "需要 curl 或 wget"
    exit 1
}

# 下载文件
download() {
    local url="$1"
    local dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        log_error "需要 curl 或 wget"
        exit 1
    fi
}

# 安装二进制
install_binary() {
    local arch="$1"
    local binary_name="hades-linux-${arch}"
    local download_url="${BASE_URL}/${binary_name}"
    local tmp_file="/tmp/hades-${arch}"

    log_info "下载 Hades ${VERSION} (${arch})..."
    log_info "URL: ${download_url}"

    if ! download "$download_url" "$tmp_file"; then
        log_error "下载失败，请检查网络连接"
        exit 1
    fi

    # 验证文件大小（至少 1MB）
    local file_size
    file_size=$(wc -c < "$tmp_file" 2>/dev/null || echo 0)
    if [ "$file_size" -lt 1048576 ]; then
        log_error "下载的文件异常（${file_size} bytes），可能不是二进制文件"
        rm -f "$tmp_file"
        exit 1
    fi

    # 停止旧服务
    if [ -x "${INIT_DIR}/hades" ]; then
        log_info "停止旧版本..."
        "${INIT_DIR}/hades" stop 2>/dev/null || true
    fi

    # 安装
    log_info "安装到 ${INSTALL_DIR}/hades..."
    mv "$tmp_file" "${INSTALL_DIR}/hades"
    chmod 755 "${INSTALL_DIR}/hades"

    log_info "二进制安装完成"
}

# 安装配置
install_config() {
    # 初始化脚本
    log_info "安装 procd 初始化脚本..."
    cat > "${INIT_DIR}/hades" << 'INITEOF'
#!/bin/sh /etc/rc.common

START=60
STOP=40
USE_PROCD=1

PROG=/usr/bin/hades
CONF_DIR=/etc/hades

start_service() {
    local config_enable config_path
    config_load hades
    config_get config_enable config enable '1'
    config_get config_path config config_path "${CONF_DIR}/config.yaml"

    if [ "$config_enable" != "1" ]; then
        echo "Hades is disabled in /etc/config/hades"
        return 1
    fi

    if [ ! -f "$config_path" ]; then
        echo "Config not found: $config_path"
        return 1
    fi

    procd_open_instance hades
    procd_set_param command "$PROG" -c "$config_path"
    procd_set_param respawn 3600 5 5
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_set_param limits nofile="65535:65535"
    procd_close_instance
}

stop_service() {
    local pid
    pid=$(pgrep -f "hades -c" 2>/dev/null | head -1)
    if [ -n "$pid" ]; then
        kill -TERM "$pid" 2>/dev/null
        sleep 2
        kill -0 "$pid" 2>/dev/null && kill -KILL "$pid" 2>/dev/null
    fi
}

service_triggers() {
    procd_add_reload_trigger "hades"
}

reload_service() {
    stop
    start
}
INITEOF
    chmod 755 "${INIT_DIR}/hades"

    # UCI 配置
    log_info "安装 UCI 配置..."
    if [ ! -f "${UCI_DIR}/hades" ]; then
        cat > "${UCI_DIR}/hades" << 'UCIEOF'
config hades 'config'
	option enable '1'
	option config_path '/etc/hades/config.yaml'
	option api_listen '127.0.0.1:9090'
	option api_secret ''
	option log_level 'info'
	option autostart '1'
UCIEOF
    fi

    # 默认配置文件
    log_info "安装默认配置..."
    mkdir -p "$CONFIG_DIR"
    if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
        cat > "${CONFIG_DIR}/config.yaml" << 'YAMLEOF'
# Hades v1.0.0 - OpenWrt 默认配置
mixed-port: 7890
allow-lan: true
bind-address: "*"
mode: rule
log-level: info
ipv6: false

external-controller: 0.0.0.0:9090
secret: ""

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - '*.lan'
  default-nameserver:
    - 223.5.5.5
    - 119.29.29.29
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  fallback:
    - tls://8.8.4.4
    - tls://1.1.1.1

sniffer:
  enable: true
  override-destination: true
  force-dns-mapping: true
  parse-pure-ip: true
  sniff:
    HTTP:
      ports: [80, 8080-8880]
    TLS:
      ports: [443, 8443]

proxies: []

proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - DIRECT
  - name: "Domestic"
    type: select
    proxies:
      - DIRECT

rules:
  - DOMAIN-SUFFIX,cn,Domestic
  - DOMAIN-KEYWORD,baidu,Domestic
  - DOMAIN-KEYWORD,taobao,Domestic
  - GEOIP,CN,Domestic
  - MATCH,Proxy
YAMLEOF
    else
        log_info "配置文件已存在，跳过"
    fi
}

# 启用服务
enable_service() {
    log_info "启用开机自启..."
    "${INIT_DIR}/hades" enable 2>/dev/null || true
}

# 显示状态
show_status() {
    echo ""
    log_info "安装完成！"
    echo ""
    echo "  版本: ${VERSION}"
    echo "  二进制: ${INSTALL_DIR}/hades"
    echo "  配置: ${CONFIG_DIR}/config.yaml"
    echo "  服务: ${INIT_DIR}/hades"
    echo ""
    echo "  常用命令:"
    echo "    /etc/init.d/hades start     # 启动"
    echo "    /etc/init.d/hades stop      # 停止"
    echo "    /etc/init.d/hades restart   # 重启"
    echo "    /etc/init.d/hades status    # 状态"
    echo "    hades -v                    # 版本"
    echo ""
    echo "  配置文件: vi ${CONFIG_DIR}/config.yaml"
    echo "  UCI 配置: uci show hades"
    echo ""
}

# 卸载
uninstall() {
    log_info "卸载 Hades..."
    "${INIT_DIR}/hades" stop 2>/dev/null || true
    "${INIT_DIR}/hades" disable 2>/dev/null || true
    rm -f "${INSTALL_DIR}/hades"
    rm -f "${INIT_DIR}/hades"
    rm -f "${UCI_DIR}/hades"
    rm -rf "${CONFIG_DIR}"
    log_info "卸载完成"
}

# 主流程
main() {
    check_root

    case "${1:-}" in
        uninstall)
            uninstall
            exit 0
            ;;
        status)
            "${INIT_DIR}/hades" status 2>/dev/null || echo "Hades 未安装"
            exit 0
            ;;
    esac

    local os
    os=$(detect_os)
    if [ "$os" != "openwrt" ]; then
        log_warn "当前系统不是 OpenWrt ($os)，继续安装..."
    fi

    local arch
    arch="${1:-$(detect_arch)}"
    log_info "目标架构: ${arch}"

    install_binary "$arch"
    install_config
    enable_service
    show_status
}

main "$@"
