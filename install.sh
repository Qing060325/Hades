#!/bin/bash
# ╔══════════════════════════════════════════════════════════════╗
# ║          Hades 一键安装脚本 v3.0                            ║
# ║   高性能代理内核 — 开箱即用                                  ║
# ║                                                              ║
# ║   用法:                                                      ║
# ║     非交互安装:  curl -fsSL <url> | sudo bash               ║
# ║     交互管理:    sudo bash install.sh                        ║
# ║     指定操作:    sudo bash install.sh [install|start|stop   ║
# ║                   |restart|update|uninstall|status|logs]    ║
# ║     用户模式:    bash install.sh --user                     ║
# ╚══════════════════════════════════════════════════════════════╝

set -euo pipefail

# ────────────────────── 配置 ──────────────────────

readonly SCRIPT_VER="3.0"
readonly REPO="Qing060325/Hades"
readonly GITHUB_API="https://api.github.com/repos/${REPO}"
readonly GITHUB_URL="https://github.com/${REPO}"
readonly GITHUB_RAW="https://raw.githubusercontent.com/${REPO}/main"
readonly FALLBACK_VERSION="v0.5.0"

# 安装路径（root 模式 vs 用户模式）
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/hades"
LOG_DIR="/var/log/hades"
SERVICE_USER="hades"
SERVICE_FILE="/etc/systemd/system/hades.service"
PID_FILE="/var/run/hades.pid"

# ────────────────────── 颜色 ──────────────────────

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

# ────────────────────── 工具函数 ──────────────────────

info()  { echo -e "${BLUE}[INFO]${NC}  $1"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
err()   { echo -e "${RED}[ERROR]${NC} $1"; }
step()  { echo -e "\n${CYAN}━━━${NC} ${BOLD}$1${NC}"; }
die()   { err "$1"; exit 1; }

read_input() {
  local prompt="$1"; local default="${2:-}"
  if [ -t 0 ]; then read -r -p "$prompt" val; else read -r val </dev/tty; fi
  echo "${val:-$default}"
}

confirm() {
  local prompt="$1"; local default="${2:-N}"
  local val; val=$(read_input "$prompt [$default] " "$default")
  [[ "$val" =~ ^[yY] ]]
}

download() {
  local url="$1" output="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL "$url" -o "$output"
  elif command -v wget &>/dev/null; then
    wget -q "$url" -O "$output"
  else
    die "需要 curl 或 wget"
  fi
}

# ────────────────────── Banner ──────────────────────

show_banner() {
  echo -e "${BOLD}${CYAN}"
  echo '  _   _           _     '
  echo ' | | | |         | |    '
  echo ' | |_| | __ _ ___| |__  '
  echo ' |  _  |/ _` / __|  _ \ '
  echo ' | | | | (_| \__ \ | | |'
  echo ' |_| |_|\__,_|___/_| |_|'
  echo -e "${NC}"
  echo -e "  ${BOLD}Hades${NC} 安装脚本 v${SCRIPT_VER}"
  echo -e "  ${DIM}高性能代理内核 — 开箱即用${NC}"
  echo ""
}

# ────────────────────── 环境检测 ──────────────────────

OS="" ARCH=""

detect_env() {
  step "检测系统环境"

  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$OS" in
    linux)   OS="linux" ;;
    darwin)  OS="darwin" ;;
    *)       die "不支持的操作系统: $(uname -s)" ;;
  esac

  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64)   ARCH="amd64" ;;
    aarch64|arm64)  ARCH="arm64" ;;
    armv7l|armhf)   ARCH="arm" ;;
    i386|i686)      ARCH="386" ;;
    *)              die "不支持的架构: $ARCH" ;;
  esac

  ok "系统: ${OS}/${ARCH}"
}

# ────────────────────── 用户模式 ──────────────────────

setup_user_mode() {
  if [ "${1:-}" = "--user" ] || [ "$(id -u)" -ne 0 ]; then
    INSTALL_DIR="$HOME/.local/bin"
    CONFIG_DIR="$HOME/.config/hades"
    LOG_DIR="$HOME/.local/var/log/hades"
    SERVICE_USER="$(whoami)"
    PID_FILE="$HOME/.local/var/run/hades.pid"
    info "用户模式: 安装至 ${INSTALL_DIR}"
  fi
}

# ────────────────────── 获取最新版本 ──────────────────────

get_latest_version() {
  local ver
  ver=$(download "${GITHUB_API}/releases/latest" /tmp/hades-release.json 2>/dev/null \
    && grep '"tag_name"' /tmp/hades-release.json | sed -E 's/.*"([^"]+)".*/\1/' \
    || echo "")
  rm -f /tmp/hades-release.json
  echo "${ver:-$FALLBACK_VERSION}"
}

# ────────────────────── 安装二进制 ──────────────────────

install_binary() {
  local version="$1"
  local binary_name="hades-${OS}-${ARCH}"
  local download_url="${GITHUB_URL}/releases/download/${version}/${binary_name}"
  local tmp_file="/tmp/${binary_name}"

  step "下载 Hades ${version}"

  local retries=0 max_retries=3 success=false
  while [ $retries -lt $max_retries ] && [ "$success" = false ]; do
    retries=$((retries + 1))
    info "尝试下载 (${retries}/${max_retries})..."

    if download "$download_url" "$tmp_file"; then
      local fsize
      fsize=$(stat -c%s "$tmp_file" 2>/dev/null || stat -f%z "$tmp_file" 2>/dev/null || echo "0")
      if [ "$fsize" -lt 500000 ]; then
        warn "文件过小 (${fsize} 字节)，可能是错误响应"
        rm -f "$tmp_file"
        [ $retries -lt $max_retries ] && sleep 2
      else
        success=true
      fi
    else
      warn "下载失败"
      [ $retries -lt $max_retries ] && sleep 2
    fi
  done

  if [ "$success" = true ]; then
    # SHA256 校验
    local sha256_url="${download_url}.sha256"
    local sha256_file="/tmp/${binary_name}.sha256"
    if download "$sha256_url" "$sha256_file" 2>/dev/null; then
      local expected actual
      expected=$(awk '{print $1}' "$sha256_file")
      if command -v sha256sum &>/dev/null; then
        actual=$(sha256sum "$tmp_file" | awk '{print $1}')
      elif command -v shasum &>/dev/null; then
        actual=$(shasum -a 256 "$tmp_file" | awk '{print $1}')
      fi
      if [ -n "$actual" ] && [ "$expected" = "$actual" ]; then
        ok "SHA256 校验通过"
      elif [ -n "$actual" ]; then
        warn "SHA256 校验不一致，但继续安装（期望: ${expected:0:16}... 实际: ${actual:0:16}...）"
      fi
      rm -f "$sha256_file"
    else
      warn "未找到 SHA256 校验文件，跳过校验"
    fi

    chmod +x "$tmp_file"
    mkdir -p "$INSTALL_DIR"
    mv "$tmp_file" "${INSTALL_DIR}/hades"
    ok "二进制已安装: ${INSTALL_DIR}/hades"
  else
    warn "预编译二进制下载失败，尝试源码构建..."
    build_from_source
  fi
}

# ────────────────────── 源码构建 ──────────────────────

build_from_source() {
  step "从源码构建"
  command -v go &>/dev/null || die "需要 Go 环境来构建: https://go.dev/dl/"

  local tmp_dir; tmp_dir=$(mktemp -d)
  git clone "https://github.com/${REPO}.git" "$tmp_dir"
  cd "$tmp_dir"
  make deps 2>/dev/null || go mod tidy
  make build 2>/dev/null || go build -o bin/hades ./cmd/hades

  mkdir -p "$INSTALL_DIR"
  mv bin/hades "${INSTALL_DIR}/hades"
  chmod +x "${INSTALL_DIR}/hades"
  rm -rf "$tmp_dir"
  ok "源码构建完成"
}

# ────────────────────── 配置 ──────────────────────

setup_config() {
  step "初始化配置"

  mkdir -p "$CONFIG_DIR" "$LOG_DIR"

  if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
    info "下载默认配置..."
    if download "${GITHUB_RAW}/configs/config.yaml" "${CONFIG_DIR}/config.yaml" 2>/dev/null; then
      local fsize
      fsize=$(stat -c%s "${CONFIG_DIR}/config.yaml" 2>/dev/null || echo "0")
      if [ "$fsize" -lt 100 ]; then
        warn "配置文件异常，生成最小配置"
        generate_minimal_config
      fi
    else
      warn "配置文件下载失败，生成最小配置"
      generate_minimal_config
    fi
    ok "配置已创建: ${CONFIG_DIR}/config.yaml"
  else
    ok "配置已存在: ${CONFIG_DIR}/config.yaml"
  fi
}

generate_minimal_config() {
  cat > "${CONFIG_DIR}/config.yaml" << 'EOF'
# Hades 配置文件
mixed-port: 7890
external-controller: 0.0.0.0:9090
mode: rule
log-level: info
dns:
  enable: true
  listen: 0.0.0.0:1053
  nameserver:
    - 223.5.5.5
    - 119.29.29.29
proxies: []
proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - DIRECT
rules:
  - GEOIP,CN,DIRECT
  - MATCH,PROXY
EOF
}

# ────────────────────── 服务管理脚本 ──────────────────────

install_ctl_script() {
  local ctl_path="${INSTALL_DIR}/hades-ctl"
  cat > "$ctl_path" << CTLEOF
#!/bin/bash
# Hades 服务管理脚本 (auto-generated)
CONFIG="${CONFIG_DIR}/config.yaml"
PID="${PID_FILE}"
LOG="${LOG_DIR}/hades.log"
BINARY="${INSTALL_DIR}/hades"

start() {
  [ -f "\$PID" ] && kill -0 \$(cat "\$PID") 2>/dev/null && echo "Hades 已在运行 (PID: \$(cat \$PID))" && return 0
  mkdir -p "\$(dirname "\$LOG")" "\$(dirname "\$PID")"
  nohup "\$BINARY" -c "\$CONFIG" > "\$LOG" 2>&1 &
  echo \$! > "\$PID"
  echo "Hades 已启动 (PID: \$(cat \$PID))"
}

stop() {
  if [ -f "\$PID" ]; then
    kill \$(cat "\$PID") 2>/dev/null; rm -f "\$PID"
    echo "Hades 已停止"
  else
    echo "Hades 未运行"
  fi
}

restart() { stop; sleep 1; start; }
status()  { [ -f "\$PID" ] && kill -0 \$(cat "\$PID") 2>/dev/null && echo "运行中 (PID: \$(cat \$PID))" || echo "未运行"; }
logs()    { tail -f "\$LOG"; }

case "\$1" in
  start)   start ;;
  stop)    stop ;;
  restart) restart ;;
  status)  status ;;
  logs)    logs ;;
  *)       echo "用法: hades-ctl {start|stop|restart|status|logs}" ;;
esac
CTLEOF
  chmod +x "$ctl_path"
  ok "管理脚本已安装: hades-ctl"
}

# ────────────────────── 系统服务 ──────────────────────

install_service() {
  step "配置系统服务"

  if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
    install_systemd_service
  elif [ "$OS" = "darwin" ]; then
    install_launchd_service
  else
    warn "未检测到支持的 init 系统，请使用 hades-ctl 管理服务"
  fi
}

install_systemd_service() {
  # 创建专用用户（root 模式）
  if [ "$SERVICE_USER" = "hades" ] && ! id hades &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin hades 2>/dev/null || true
    chown -R hades:hades "$CONFIG_DIR" "$LOG_DIR"
  fi

  cat > "$SERVICE_FILE" << EOF
[Unit]
Description=Hades Proxy Kernel
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=$( [ "$SERVICE_USER" = "hades" ] && echo "hades" || echo "$(id -gn)" )
ExecStart=${INSTALL_DIR}/hades -c ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576
WorkingDirectory=${CONFIG_DIR}

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${CONFIG_DIR} ${LOG_DIR}

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable hades
  ok "systemd 服务已安装: systemctl {start|stop|status} hades"
}

install_launchd_service() {
  local plist_path="$HOME/Library/LaunchAgents/com.hades.plist"
  mkdir -p "$(dirname "$plist_path")"
  cat > "$plist_path" << EOF
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
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/hades.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/hades-error.log</string>
</dict>
</plist>
EOF
  launchctl load "$plist_path" 2>/dev/null || true
  ok "launchd 服务已安装"
}

# ────────────────────── 安装主流程 ──────────────────────

do_install() {
  show_banner
  setup_user_mode "${1:-}"
  detect_env

  local version
  version=$(get_latest_version)
  ok "最新版本: ${version}"

  install_binary "$version"
  setup_config
  install_ctl_script

  # 询问是否安装系统服务
  echo ""
  if confirm "是否安装系统服务（开机自启）？ [Y/n] " "Y"; then
    install_service
  else
    warn "跳过系统服务安装，可使用 hades-ctl 手动管理"
  fi

  show_install_success
}

# ────────────────────── 管理操作 ──────────────────────

do_start() {
  if command -v systemctl &>/dev/null && [ -f "$SERVICE_FILE" ]; then
    systemctl start hades && ok "Hades 已启动"
  elif [ -f "${INSTALL_DIR}/hades-ctl" ]; then
    "${INSTALL_DIR}/hades-ctl" start
  else
    nohup "${INSTALL_DIR}/hades" -c "${CONFIG_DIR}/config.yaml" > "${LOG_DIR}/hades.log" 2>&1 &
    ok "Hades 已启动 (PID: $!)"
  fi
}

do_stop() {
  if command -v systemctl &>/dev/null && [ -f "$SERVICE_FILE" ]; then
    systemctl stop hades && ok "Hades 已停止"
  elif [ -f "${INSTALL_DIR}/hades-ctl" ]; then
    "${INSTALL_DIR}/hades-ctl" stop
  else
    [ -f "$PID_FILE" ] && kill "$(cat "$PID_FILE")" 2>/dev/null && rm -f "$PID_FILE"
    ok "Hades 已停止"
  fi
}

do_restart() {
  do_stop; sleep 1; do_start
  ok "Hades 已重启"
}

do_update() {
  step "更新 Hades"
  detect_env

  local version
  version=$(get_latest_version)
  info "目标版本: ${version}"

  # 停止服务
  do_stop 2>/dev/null || true

  # 备份
  local backup_dir="/tmp/hades-backup-$(date +%s)"
  mkdir -p "$backup_dir"
  [ -f "${CONFIG_DIR}/config.yaml" ] && cp "${CONFIG_DIR}/config.yaml" "$backup_dir/"

  # 安装新版本
  install_binary "$version"

  # 恢复配置
  [ -f "$backup_dir/config.yaml" ] && cp "$backup_dir/config.yaml" "${CONFIG_DIR}/config.yaml"
  rm -rf "$backup_dir"

  # 启动
  do_start
  ok "更新完成"
}

do_uninstall() {
  echo -e "\n${RED}⚠️  即将卸载 Hades 及所有配置${NC}"
  if ! confirm "确定要卸载吗？此操作不可恢复 [y/N] " "N"; then
    info "已取消"; return
  fi

  step "卸载 Hades"

  # 停止服务
  if command -v systemctl &>/dev/null && [ -f "$SERVICE_FILE" ]; then
    systemctl stop hades 2>/dev/null || true
    systemctl disable hades 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
  fi
  if [ "$OS" = "darwin" ]; then
    launchctl unload "$HOME/Library/LaunchAgents/com.hades.plist" 2>/dev/null || true
    rm -f "$HOME/Library/LaunchAgents/com.hades.plist"
  fi

  # 删除文件
  rm -f "${INSTALL_DIR}/hades" "${INSTALL_DIR}/hades-ctl"
  rm -rf "$CONFIG_DIR" "$LOG_DIR" "$PID_FILE"

  # 删除用户（如果是 root 且用户存在）
  if [ "$(id -u)" -eq 0 ] && id hades &>/dev/null; then
    userdel hades 2>/dev/null || true
  fi

  ok "Hades 已完全卸载"
}

do_logs() {
  if command -v systemctl &>/dev/null && [ -f "$SERVICE_FILE" ]; then
    journalctl -u hades -f --no-pager -n 100
  else
    tail -f "${LOG_DIR}/hades.log"
  fi
}

# ────────────────────── 状态展示 ──────────────────────

show_status() {
  echo ""
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

  local running=false
  if command -v systemctl &>/dev/null && systemctl is-active --quiet hades 2>/dev/null; then
    running=true
  elif [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
    running=true
  fi

  if $running; then
    echo -e "  状态: ${GREEN}● 运行中${NC}"
  elif [ -f "${INSTALL_DIR}/hades" ]; then
    echo -e "  状态: ${YELLOW}○ 已停止${NC}"
  else
    echo -e "  状态: ${RED}✖ 未安装${NC}"
  fi

  if [ -f "${INSTALL_DIR}/hades" ]; then
    local ver
    ver=$("${INSTALL_DIR}/hades" -v 2>/dev/null | head -1 || echo "unknown")
    echo -e "  版本: ${ver}"
    echo ""
    echo -e "  二进制: ${CYAN}${INSTALL_DIR}/hades${NC}"
    echo -e "  配置:   ${CYAN}${CONFIG_DIR}/config.yaml${NC}"
    echo -e "  日志:   ${CYAN}${LOG_DIR}/hades.log${NC}"
    echo ""
    echo -e "  混合端口: 7890 | API 端口: 9090 | DNS 端口: 1053"
  fi

  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

show_install_success() {
  echo ""
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}${BOLD}  🎉 Hades 安装成功！${NC}"
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo -e "  🚀 快速开始:"
  echo -e "     前台运行:  ${CYAN}hades -c ${CONFIG_DIR}/config.yaml${NC}"
  echo -e "     服务管理:  ${CYAN}hades-ctl start${NC}"
  echo -e "     系统服务:  ${CYAN}systemctl start hades${NC}"
  echo ""
  echo -e "  📋 常用命令:"
  echo -e "     查看状态:  ${CYAN}bash install.sh status${NC}"
  echo -e "     查看日志:  ${CYAN}bash install.sh logs${NC}"
  echo -e "     更新版本:  ${CYAN}bash install.sh update${NC}"
  echo -e "     卸载:      ${CYAN}bash install.sh uninstall${NC}"
  echo ""
  echo -e "  ${DIM}配置文件: ${CONFIG_DIR}/config.yaml${NC}"
  echo -e "  ${DIM}日志目录: ${LOG_DIR}${NC}"
  echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# ────────────────────── 交互菜单 ──────────────────────

show_menu() {
  echo ""
  echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
  echo -e "${CYAN}║${NC}      ${BOLD}Hades 管理工具 v${SCRIPT_VER}${NC}           ${CYAN}║${NC}"
  echo -e "${CYAN}╠══════════════════════════════════════╣${NC}"

  local status_icon
  if command -v systemctl &>/dev/null && systemctl is-active --quiet hades 2>/dev/null; then
    status_icon="${GREEN}● 运行中${NC}"
  elif [ -f "${INSTALL_DIR}/hades" ]; then
    status_icon="${YELLOW}○ 已停止${NC}"
  else
    status_icon="${RED}✖ 未安装${NC}"
  fi

  echo -e "${CYAN}║${NC}  状态: ${status_icon}                    ${CYAN}║${NC}"
  echo -e "${CYAN}╠══════════════════════════════════════╣${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}1${NC}) 安装/更新 Hades                  ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}2${NC}) 启动服务                           ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}3${NC}) 停止服务                           ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}4${NC}) 重启服务                           ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}5${NC}) 更新到最新版本                     ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}6${NC}) 查看日志                           ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${GREEN}7${NC}) 查看状态                           ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${RED}8${NC}) 卸载                               ${CYAN}║${NC}"
  echo -e "${CYAN}║${NC}  ${DIM}0${NC}) 退出                               ${CYAN}║${NC}"
  echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"
  echo -n "请选择 [0-8]: "
}

interactive() {
  show_banner
  while true; do
    show_menu
    local choice
    if [ -t 0 ]; then read -r choice; else read -r choice </dev/tty; fi
    case "$choice" in
      1) do_install ;;
      2) do_start ;;
      3) do_stop ;;
      4) do_restart ;;
      5) do_update ;;
      6) do_logs ;;
      7) show_status ;;
      8) do_uninstall ;;
      0) info "再见！"; exit 0 ;;
      *) warn "无效选项" ;;
    esac
  done
}

# ────────────────────── 入口 ──────────────────────

main() {
  # 用户模式
  if [[ "${1:-}" == "--user" ]]; then
    shift
    setup_user_mode "--user"
  fi

  case "${1:-}" in
    install)    do_install "${2:-}" ;;
    start)      do_start ;;
    stop)       do_stop ;;
    restart)    do_restart ;;
    update)     do_update ;;
    uninstall)  do_uninstall ;;
    status)     show_status ;;
    logs)       do_logs ;;
    version|-v) echo "Hades install script v${SCRIPT_VER}" ;;
    help|-h)
      echo "用法: $0 [命令] [选项]"
      echo ""
      echo "命令:"
      echo "  install    安装 Hades（默认操作）"
      echo "  start      启动服务"
      echo "  stop       停止服务"
      echo "  restart    重启服务"
      echo "  update     更新到最新版本"
      echo "  uninstall  卸载 Hades"
      echo "  status     查看运行状态"
      echo "  logs       查看实时日志"
      echo "  version    显示脚本版本"
      echo ""
      echo "选项:"
      echo "  --user     安装到用户目录（无需 root）"
      echo ""
      echo "不带参数运行将进入交互菜单"
      ;;
    *)
      if [ -t 0 ]; then
        interactive
      else
        do_install
      fi
      ;;
  esac
}

main "$@"
