# Hades - 高性能代理内核

<p align="center">
  <img src="https://img.shields.io/badge/version-v0.2.0-blue" alt="version">
  <img src="https://img.shields.io/badge/go-1.21+-00ADD8" alt="go">
  <img src="https://img.shields.io/badge/license-MIT-green" alt="license">
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey" alt="platform">
</p>

<p align="center">
  <b>一键安装 · 傻瓜式配置 · 开箱即用 · Clash 兼容</b>
</p>

---

## 简介

**Hades** 是一个使用 Go 语言编写的高性能代理内核，类似 mihomo/Clash，追求极致性能。

### ✨ v0.2.0 新特性

| 特性 | 描述 |
|------|------|
| 🚀 **一键安装** | 跨平台安装脚本，自动检测系统和架构 |
| 📦 **傻瓜式配置** | 交互式配置向导，开箱即用 |
| 🔄 **Clash 兼容** | 完全兼容 Clash/Mihomo YAML 配置格式 |
| 🆕 **Hysteria2** | 新增基于 QUIC 的高性能代理协议 |
| 🆕 **TUIC** | 新增基于 QUIC 的低延迟代理协议 |
| 🆕 **WireGuard** | 新增现代化 VPN 协议支持 |

---

## 快速开始

### 一键安装

```bash
# Linux/macOS
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

### 手动安装

```bash
git clone https://github.com/Qing060325/Hades.git
cd Hades
make deps && make build
./bin/hades -c configs/config.yaml
```

---

## 支持的协议

| 协议 | 入站 | 出站 | UDP | 说明 |
|------|:----:|:----:|:---:|------|
| HTTP | ✅ | ✅ | ❌ | HTTP CONNECT 代理 |
| SOCKS5 | ✅ | ✅ | ✅ | SOCKS5 代理 |
| Shadowsocks | ❌ | ✅ | ✅ | AEAD-2022 加密 |
| VMess | ❌ | ✅ | ✅ | WebSocket/gRPC |
| VLESS | ❌ | ✅ | ✅ | XTLS-RPRX-Vision |
| Trojan | ❌ | ✅ | ✅ | WebSocket/gRPC |
| **Hysteria2** | ❌ | ✅ | ✅ | 🆕 QUIC 高性能 |
| **TUIC** | ❌ | ✅ | ✅ | 🆕 QUIC 低延迟 |
| **WireGuard** | ❌ | ✅ | ✅ | 🆕 现代 VPN |

---

## Clash YAML 兼容

Hades **完全兼容** Clash/Mihomo 配置格式：

```yaml
mixed-port: 7890
allow-lan: true
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

  - name: "tuic-node"
    type: tuic
    server: example.com
    port: 443
    uuid: "uuid"
    password: "password"

proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - hysteria2-node
      - DIRECT

rules:
  - GEOIP,CN,DIRECT
  - MATCH,proxy
```

---

## 新协议配置

### Hysteria2

```yaml
- name: "hysteria2-node"
  type: hysteria2
  server: example.com
  port: 443
  password: "password"
  sni: example.com
  up: "100 Mbps"
  down: "100 Mbps"
```

### TUIC

```yaml
- name: "tuic-node"
  type: tuic
  server: example.com
  port: 443
  uuid: "uuid"
  password: "password"
  congestion-controller: bbr
  udp-relay-mode: native
```

### WireGuard

```yaml
- name: "wireguard-node"
  type: wireguard
  server: example.com
  port: 51820
  private-key: "private-key"
  public-key: "public-key"
  mtu: 1400
```

---

## 核心功能

- ✅ 混合端口监听 (HTTP + SOCKS5)
- ✅ TUN 模式 (跨平台)
- ✅ 规则引擎 (DOMAIN/IPCIDR/GEOIP)
- ✅ DNS 系统 (Fake-IP/DoH/DoT)
- ✅ 代理组 (Select/URLTest/Fallback/LoadBalance)
- ✅ RESTful API
- ✅ 流量嗅探 (TLS/HTTP/QUIC)
- ✅ 连接池管理
- ✅ 流量统计

---

## 文档

- [完整文档](docs/README.md)
- [配置指南](docs/CONFIGURATION.md)
- [协议说明](docs/PROTOCOLS.md)
- [配置示例](docs/EXAMPLES.md)

---

## 常用命令

```bash
# 启动
hades -c /etc/hades/config.yaml

# 调试模式
hades -c config.yaml -d

# 服务管理
hades-ctl start    # 启动
hades-ctl stop     # 停止
hades-ctl restart  # 重启
hades-ctl status   # 状态
hades-ctl logs     # 日志
```

---

## 更新日志

### v0.2.0 (2024-04)

- ✨ 新增 Hysteria2 协议支持
- ✨ 新增 TUIC 协议支持
- ✨ 新增 WireGuard 协议支持
- ✨ 实现 Clash YAML 完全兼容
- ✨ 一键安装脚本
- ✨ 交互式配置向导
- 📝 完善文档和配置示例

### v0.1.0

- 🎉 初始版本
- ✅ 支持 HTTP/SOCKS5/SS/VMess/VLESS/Trojan

---

## 致谢

参考项目：
- [mihomo](https://github.com/MetaCubeX/mihomo)
- [clash](https://github.com/Dreamacro/clash)

---

## 许可证

[MIT License](LICENSE)
