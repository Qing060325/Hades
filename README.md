<div align="center">

# Hades

**高性能代理内核 — Go 语言编写，追求极致性能**

[![Go Version](https://img.shields.io/badge/go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Version](https://img.shields.io/github/v/release/Qing060325/Hades?color=blue)](https://github.com/Qing060325/Hades/releases)
[![License](https://img.shields.io/github/license/Qing060325/Hades?color=green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey?logo=data:image/svg+xml;base64,PHN2Zy8+)]()
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue?logo=docker&logoColor=white)]()

**一键安装 · 傻瓜式配置 · 开箱即用 · Clash 兼容**

</div>

---

## 简介

**Hades** 是一个使用 Go 语言编写的高性能代理内核，类似 [mihomo](https://github.com/MetaCubeX/mihomo) / [Clash](https://github.com/Dreamacro/clash)，追求极致性能和简洁体验。

### 核心特性

| 特性 | 描述 |
|------|------|
| 🚀 **一键安装** | 跨平台安装脚本，自动检测系统和架构 |
| 📦 **傻瓜式配置** | 交互式配置向导，开箱即用 |
| 🔄 **Clash 兼容** | 完全兼容 Clash/Mihomo YAML 配置格式 |
| ⚡ **高性能** | Go 编写，零拷贝 I/O，协程池优化 |
| 🔒 **安全可靠** | TLS 1.3 / AEAD 加密 / 安全默认配置 |

### 协议支持

| 协议 | 入站 | 出站 | UDP | 说明 |
|------|:----:|:----:|:---:|------|
| HTTP | ✅ | ✅ | ❌ | HTTP CONNECT 代理 |
| SOCKS5 | ✅ | ✅ | ✅ | SOCKS5 代理 |
| Shadowsocks | ❌ | ✅ | ✅ | AEAD-2022 加密 |
| VMess | ❌ | ✅ | ✅ | TLS/WebSocket/gRPC |
| VLESS | ❌ | ✅ | ✅ | XTLS-RPRX-Vision |
| Trojan | ❌ | ✅ | ✅ | TLS/WebSocket/gRPC |
| Hysteria2 | ❌ | ✅ | ✅ | 基于 QUIC 的高性能协议 |
| TUIC | ❌ | ✅ | ✅ | 基于 QUIC 的低延迟协议 |
| WireGuard | ❌ | ✅ | ✅ | 现代 VPN 协议 |

---

## 快速开始

### 一键安装

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | bash
```

### Docker

```bash
docker run -d \
  --name hades \
  -p 7890:7890 \
  -p 9090:9090 \
  -v /path/to/config:/etc/hades \
  ghcr.io/qing060325/hades:latest
```

### 从源码构建

```bash
git clone https://github.com/Qing060325/Hades.git
cd Hades
make deps && make build
./bin/hades -c configs/config.yaml
```

---

## 配置示例

Hades 完全兼容 Clash/Mihomo 配置格式：

```yaml
mixed-port: 7890
allow-lan: false
mode: rule

dns:
  enable: true
  enhanced-mode: fake-ip
  nameserver:
    - https://dns.alidns.com/dns-query

proxies:
  - name: "hysteria2-node"
    type: hysteria2
    server: example.com
    port: 443
    password: "password"
    sni: example.com
    up: "100 Mbps"
    down: "100 Mbps"

  - name: "tuic-node"
    type: tuic
    server: example.com
    port: 443
    uuid: "uuid"
    password: "password"
    congestion-controller: bbr

  - name: "wireguard-node"
    type: wireguard
    server: example.com
    port: 51820
    private-key: "your-private-key"
    public-key: "server-public-key"
    mtu: 1400

proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - hysteria2-node
      - tuic-node
      - DIRECT

rules:
  - GEOIP,CN,DIRECT
  - MATCH,proxy
```

更多配置示例请参考 [配置指南](docs/CONFIGURATION.md) 和 [示例集合](docs/EXAMPLES.md)。

---

## 核心架构

```
Hades
├── cmd/
│   ├── hades/              # 主程序入口
│   └── hades-sub2config/   # 订阅转换工具
├── internal/
│   ├── app/                # 应用生命周期管理
│   ├── config/             # 配置解析 (Clash YAML 兼容)
│   └── version/            # 版本信息
├── pkg/
│   ├── api/                # RESTful 管理 API
│   ├── core/
│   │   ├── adapter/        # 协议适配器 (SS/VMess/VLESS/Trojan/Hy2/TUIC/WG)
│   │   ├── dialer/         # 出站拨号
│   │   ├── dns/            # DNS 系统 (Fake-IP/DoH/DoT)
│   │   ├── group/          # 代理组 (Select/URLTest/Fallback/LoadBalance)
│   │   ├── listener/       # 入站监听 (HTTP/SOCKS5/Mixed/TUN)
│   │   └── rules/          # 规则引擎 (DOMAIN/IPCIDR/GEOIP)
│   ├── perf/               # 性能优化 (零拷贝/协程池/Buffer池)
│   ├── sniffer/            # 流量嗅探 (TLS/HTTP/QUIC)
│   ├── stats/              # 流量统计
│   └── transport/          # 传输层 (TCP/WebSocket/gRPC/连接池)
├── configs/                # 配置文件
├── docs/                   # 文档
├── install.sh              # 一键安装脚本
├── hades.sh                # 服务管理脚本
└── Makefile                # 构建系统
```

---

## 命令参考

```bash
# 启动
hades -c /etc/hades/config.yaml

# 调试模式
hades -c config.yaml -d

# 服务管理 (通过 hades-ctl)
hades-ctl start     # 启动服务
hades-ctl stop      # 停止服务
hades-ctl restart   # 重启服务
hades-ctl status    # 查看状态
hades-ctl logs      # 查看日志
```

---

## 文档

| 文档 | 说明 |
|------|------|
| [配置指南](docs/CONFIGURATION.md) | 完整配置参数说明 |
| [协议说明](docs/PROTOCOLS.md) | 各协议详细配置 |
| [配置示例](docs/EXAMPLES.md) | 常用场景配置模板 |
| [部署指南](DEPLOYMENT.md) | 生产环境部署说明 |

---

## 更新日志

### v0.2.0

- ✨ 新增 Hysteria2 协议支持
- ✨ 新增 TUIC 协议支持
- ✨ 新增 WireGuard 协议支持
- ✨ 实现 Clash YAML 完全兼容
- ✨ 一键安装脚本与交互式配置向导
- 🔒 TLS 1.3 安全连接支持
- 🔧 API 安全加固 (Authorization Header 认证)
- 🐛 修复 Fake-IP 池溢出、LRU 缓存淘汰等多个问题

### v0.1.0

- 🎉 初始版本
- ✅ 支持 HTTP / SOCKS5 / Shadowsocks / VMess / VLESS / Trojan

---

## 致谢

参考项目：
- [mihomo](https://github.com/MetaCubeX/mihomo) — Clash Meta 重写版
- [clash](https://github.com/Dreamacro/clash) — 原始 Clash 项目

---

## 许可证

[MIT License](LICENSE)
