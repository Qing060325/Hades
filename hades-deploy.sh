#!/bin/bash
# Hades 一键部署脚本 (国内优化版)
# 用法: bash deploy-hades.sh [install|uninstall|status]
set -euo pipefail

HADES_VERSION="v0.5.0"
GO_VERSION="1.23.4"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/hades"
LOG_DIR="/var/log/hades"
SERVICE_NAME="hades"
REPO="Qing060325/Hades"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[OK]${NC}   $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()   { echo -e "${RED}[ERR]${NC}  $1"; exit 1; }

cmd_install() {
    echo -e "\n${CYAN}═══ Hades 国内优化部署 ═══${NC}\n"

    # 1. 检查是否已安装
    if command -v hades &>/dev/null; then
        local cur_ver=$(hades -v 2>&1 | head -1)
        warn "已安装: $cur_ver"
        read -p "覆盖安装? [y/N] " ans
        [[ "$ans" != "y" && "$ans" != "Y" ]] && exit 0
    fi

    # 2. 安装 Go (如果需要)
    if ! command -v go &>/dev/null || ! go version &>/dev/null; then
        info "安装 Go ${GO_VERSION}..."
        # dl.google.com 在国内可达，比镜像站更稳定
        wget -q --show-progress --timeout=60 \
            "https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz" \
            -O /tmp/go-hades.tar.gz || err "Go 下载失败"
        tar -C /usr/local -xzf /tmp/go-hades.tar.gz
        export PATH=$PATH:/usr/local/go/bin
        # 写入 profile
        if ! grep -q '/usr/local/go/bin' /etc/profile.d/go.sh 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
        fi
        ok "Go $(go version | awk '{print $3}') 已安装"
    else
        ok "Go $(go version | awk '{print $3}') 已存在"
    fi

    # 3. 克隆源码 (shallow clone)
    info "克隆 Hades 源码..."
    rm -rf /tmp/hades-build 2>/dev/null
    git clone --depth 1 "https://github.com/${REPO}.git" /tmp/hades-build \
        || err "git clone 失败，请检查网络"
    ok "源码克隆完成"

    # 4. 编译 (goproxy.cn 加速依赖下载)
    info "编译中..."
    export PATH=$PATH:/usr/local/go/bin
    export GOPROXY=https://goproxy.cn,direct
    cd /tmp/hades-build
    go build -o hades ./cmd/hades || err "编译失败"
    ok "编译完成 ($(ls -lh hades | awk '{print $5}'))"

    # 5. 安装二进制
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    cp hades ${INSTALL_DIR}/hades
    chmod +x ${INSTALL_DIR}/hades
    ok "二进制已安装到 ${INSTALL_DIR}/hades"

    # 6. 配置目录
    mkdir -p ${CONFIG_DIR} ${LOG_DIR}
    if [ ! -f ${CONFIG_DIR}/config.yaml ]; then
        cp configs/config.yaml ${CONFIG_DIR}/config.yaml
        ok "默认配置已写入 ${CONFIG_DIR}/config.yaml"
    else
        warn "配置文件已存在，跳过覆盖"
    fi

    # 7. systemd 服务
    cat > /etc/systemd/system/${SERVICE_NAME}.service << SVCEOF
[Unit]
Description=Hades Proxy Service
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/hades -c ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SVCEOF

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}
    systemctl start ${SERVICE_NAME}
    sleep 1

    # 8. 验证
    if systemctl is-active --quiet ${SERVICE_NAME}; then
        ok "服务已启动并设为开机自启"
    else
        err "服务启动失败，运行 journalctl -u ${SERVICE_NAME} 查看日志"
    fi

    # 清理
    rm -rf /tmp/hades-build /tmp/go-hades.tar.gz 2>/dev/null

    echo -e "\n${GREEN}═══ 部署完成 ═══${NC}"
    echo -e "  版本:    $(hades -v 2>&1 | head -1)"
    echo -e "  代理:    :7890 (HTTP/SOCKS5)"
    echo -e "  控制API: 127.0.0.1:9090"
    echo -e "  DNS:     :1053"
    echo -e "  配置:    ${CONFIG_DIR}/config.yaml"
    echo -e "  日志:    journalctl -u ${SERVICE_NAME} -f"
    echo -e ""
    echo -e "  编辑配置后重启: systemctl restart ${SERVICE_NAME}"
}

cmd_uninstall() {
    info "卸载 Hades..."
    systemctl stop ${SERVICE_NAME} 2>/dev/null || true
    systemctl disable ${SERVICE_NAME} 2>/dev/null || true
    rm -f /etc/systemd/system/${SERVICE_NAME}.service
    rm -f ${INSTALL_DIR}/hades
    systemctl daemon-reload
    read -p "删除配置文件 ${CONFIG_DIR}? [y/N] " ans
    if [[ "$ans" == "y" || "$ans" == "Y" ]]; then
        rm -rf ${CONFIG_DIR} ${LOG_DIR}
        ok "配置已删除"
    fi
    ok "Hades 已卸载"
}

cmd_status() {
    if systemctl is-active --quiet ${SERVICE_NAME}; then
        echo -e "${GREEN}●${NC} Hades 运行中"
        hades -v 2>&1 | head -1
        echo ""
        ss -tlnp | grep -E '7890|9090|1053' || true
    else
        echo -e "${RED}●${NC} Hades 未运行"
    fi
}

case "${1:-install}" in
    install)    cmd_install ;;
    uninstall)  cmd_uninstall ;;
    status)     cmd_status ;;
    *)          echo "用法: $0 [install|uninstall|status]" ;;
esac
