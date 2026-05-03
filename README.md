<div align="center">

# 🔱 Hades

**高性能代理内核 — Go 语言编写**

追求极致性能 · 兼容 Clash 配置 · 开箱即用

[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-v1.0.0-blue)](https://github.com/Qing060325/Hades/releases)
[![License](https://img.shields.io/github/license/Qing060325/Hades?color=green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey)]()

[一键安装](#快速开始) · [配置指南](#配置示例) · [Web 面板](https://github.com/Qing060325/Hyperion)

</div>

---

## 📖 简介

**Hades** 是一个使用 Go 语言编写的高性能代理内核，完全兼容 Clash/Mihomo YAML 配置格式，支持 18 种代理协议，内置 DNS-over-HTTPS/TLS、GeoSite/GeoIP 匹配、eBPF 加速、Prometheus 监控等企业级特性。

### 🎯 为什么选择 Hades？

| 特性 | 说明 |
|------|------|
| 🚀 **高性能** | 零拷贝 I/O、协程池、splice 系统调用、eBPF 加速 |
| 📦 **开箱即用** | 一键安装脚本，交互式配置向导 |
| 🔄 **Clash 兼容** | 完全兼容 Clash/Mihomo 配置，支持别名字段 |
| 🌐 **18 种协议** | SS / VMess / VLESS / Trojan / Hysteria2 / TUIC / WireGuard / AnyTLS / MASQUE 等 |
| 🔒 **DNS 增强** | DoH / DoT / Fake-IP / GeoSite 域名匹配 |
| 📊 **可观测性** | Prometheus 指标、连接跟踪、实时日志流 |
| 🎨 **Web 管理** | 配合 [Hyperion](https://github.com/Qing060325/Hyperion) 面板 |

---

## 📦 配套项目

<div align="center">

| 项目 | 类型 | 说明 |
|:--:|:--:|:--|
| **Hades** | 代理内核 | 本仓库，高性能代理核心 |
| **[Hyperion](https://github.com/Qing060325/Hyperion)** | Web 面板 | 现代化管理界面，推荐搭配使用 |

**Hades + Hyperion = 完整的代理解决方案**

</div>

---

## 🚀 快速开始

### 方式一：一键安装（推荐）

```bash
# 非交互式安装（适合自动化部署）
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | sudo bash

# 交互式菜单（可选操作）
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | sudo bash -s -- --interactive

# 或使用子命令
sudo install.sh install     # 安装
sudo install.sh start       # 启动
sudo install.sh stop        # 停止
sudo install.sh restart     # 重启
sudo install.sh update      # 更新
sudo install.sh status      # 状态
sudo install.sh logs        # 日志
sudo install.sh uninstall   # 卸载

# 非根用户安装
./install.sh install --user
```

> 📌 安装脚本 v3.0：自动检测平台架构、SHA256 校验、systemd/launchd 服务、安全加固。

### 方式二：Docker

```bash
docker run -d \
  --name hades \
  --restart unless-stopped \
  -p 7890:7890 \
  -p 9090:9090 \
  -v /path/to/config:/etc/hades \
  ghcr.io/qing060325/hades:latest
```

### 方式三：手动安装

```bash
# 下载最新版本
wget https://github.com/Qing060325/Hades/releases/latest/download/hades-linux-amd64
chmod +x hades-linux-amd64
sudo mv hades-linux-amd64 /usr/local/bin/hades

# 创建配置目录
sudo mkdir -p /etc/hades

# 启动
hades -c /etc/hades/config.yaml
```

---

## 🎨 配合 Hyperion 使用

[Hades](https://github.com/Qing060325/Hades) + [Hyperion](https://github.com/Qing060325/Hyperion) 是最佳组合：

```bash
# 1. 安装 Hades
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | sudo bash

# 2. 安装 Hyperion Web 面板
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hyperion/main/install.sh | sudo bash

# 3. 访问 http://localhost:8080 管理 Hades
```

Hyperion 功能预览：
- 📊 实时流量监控仪表盘
- 🌐 可视化代理管理
- 🔗 连接追踪与管理
- 📋 规则编辑
- 📝 实时日志
- ⚙️ 系统设置

---

## ⚙️ 配置示例

Hades 完全兼容 Clash/Mihomo 配置格式：

```yaml
# 基础配置
mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090

# DNS 配置
dns:
  enable: true
  listen: 127.0.0.1:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  fallback:
    - tls://8.8.4.4
    - tls://1.1.1.1

# 代理节点
proxies:
  - name: "香港节点"
    type: vmess
    server: example.com
    port: 443
    uuid: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    alterId: 0
    cipher: auto
    tls: true
    network: ws
    ws-opts:
      path: /path

  - name: "美国 Hysteria2"
    type: hysteria2
    server: example.com
    port: 443
    password: your-password
    sni: example.com
    up: "100 Mbps"
    down: "100 Mbps"

# 代理组
proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - "香港节点"
      - "美国 Hysteria2"
      - DIRECT

  - name: "Auto"
    type: url-test
    url: http://www.gstatic.com/generate_204
    interval: 300
    proxies:
      - "香港节点"
      - "美国 Hysteria2"

# 规则
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
  - DOMAIN-KEYWORD,google,Proxy
  - GEOIP,CN,DIRECT
  - GEOSITE,google,Proxy
  - MATCH,Proxy
```

---

## 🔧 协议支持

| 协议 | 入站 | 出站 | UDP | 特点 |
|------|:--:|:--:|:--:|------|
| HTTP | ✅ | ✅ | ❌ | HTTP CONNECT 代理 |
| SOCKS5 | ✅ | ✅ | ✅ | 标准 SOCKS5 代理 |
| Shadowsocks | ❌ | ✅ | ✅ | AEAD-2022 加密 |
| VMess | ❌ | ✅ | ✅ | V2Ray 协议，AEAD |
| VLESS | ❌ | ✅ | ✅ | XTLS-RPRX-Vision |
| Trojan | ❌ | ✅ | ✅ | TLS 伪装 |
| Hysteria2 | ❌ | ✅ | ✅ | 基于 QUIC，抗丢包 |
| TUIC | ❌ | ✅ | ✅ | 基于 QUIC，低延迟 |
| WireGuard | ❌ | ✅ | ✅ | 现代 VPN 协议 |
| Snell | ❌ | ✅ | ✅ | Surru 混淆协议 |
| SSH | ❌ | ✅ | ❌ | SSH 隧道代理 |
| Mieru | ❌ | ✅ | ✅ | 端口跳跃，抗封锁 |
| AnyTLS | ❌ | ✅ | ✅ | TLS 填充混淆 |
| MASQUE | ❌ | ✅ | ✅ | HTTP/3 CONNECT-UDP |
| Trust-Tunnel | ❌ | ✅ | ✅ | WebSocket/gRPC 隧道 |
| Sudoku | ❌ | ✅ | ✅ | 自定义加密协议 |
| AmneziaWG | ❌ | ✅ | ✅ | 抗审查 WireGuard |
| Sing-Mux | ❌ | ✅ | ✅ | 多路复用，带宽聚合 |

---

## 🖥️ 命令参考

```bash
# 基础命令
hades -c /etc/hades/config.yaml              # 启动
hades -c /etc/hades/config.yaml -d           # 调试模式
hades -v                                     # 查看版本

# 服务管理（使用安装脚本）
sudo install.sh start                        # 启动服务
sudo install.sh stop                         # 停止服务
sudo install.sh restart                      # 重启服务
sudo install.sh status                       # 查看状态
sudo install.sh logs                         # 查看日志
sudo install.sh update                       # 更新版本
sudo install.sh uninstall                    # 卸载
```

---

## 🏗️ 项目结构

```
Hades/
├── cmd/
│   ├── hades/              # 主程序入口
│   └── hades-sub2config/   # 订阅转配置工具
├── internal/
│   ├── app/                # 应用生命周期管理
│   ├── config/             # 配置解析（Clash 兼容层）
│   ├── subscription/       # 订阅管理
│   └── version/            # 版本信息
├── pkg/
│   ├── api/                # RESTful API + Prometheus
│   ├── component/
│   │   ├── geodata/        # GeoSite 域名匹配
│   │   ├── mmdb/           # GeoIP 数据库
│   │   ├── fakeip/         # Fake-IP 池
│   │   ├── cidr/           # CIDR Trie
│   │   ├── sniffer/        # 协议嗅探器
│   │   └── process/        # 进程检测
│   ├── core/
│   │   ├── adapter/        # 协议适配器（18种）
│   │   ├── dialer/         # 出站拨号
│   │   ├── dns/            # DNS 系统（DoH/DoT/Fake-IP）
│   │   ├── group/          # 代理组（select/url-test/fallback/load-balance/relay）
│   │   ├── listener/       # 入站监听（mixed/http/socks/redir/tproxy/tun）
│   │   ├── proxyprovider/  # 远程代理提供者
│   │   ├── rules/          # 规则引擎 + 规则集 Provider
│   │   └── tunnel/         # 流量调度中枢
│   ├── ebpf/               # eBPF 加速
│   ├── perf/
│   │   ├── pool/           # Buffer/协程池
│   │   └── zerocopy/       # 零拷贝 I/O
│   ├── sniffer/            # 协议嗅探
│   ├── stats/              # 流量统计 + Prometheus
│   └── transport/          # 传输层（WebSocket/gRPC/KCP/Reality/SSH）
├── configs/                # 配置示例
├── test/                   # 集成测试
├── install.sh              # 统一安装/管理脚本 (v3.0)
└── Makefile                # 构建系统
```

---

## 📊 性能优化

| 优化项 | 说明 |
|--------|------|
| 零拷贝 I/O | splice/sendfile 系统调用，减少内存拷贝 |
| 协程池 | 高效并发处理，控制 goroutine 数量 |
| Buffer 池 | sync.Pool 复用缓冲区，减少 GC 压力 |
| 连接跟踪 | 原子操作 + sync.Map，无锁统计 |
| eBPF 加速 | 内核态规则匹配（Linux） |
| DNS 缓存 | LRU 缓存 + Fake-IP 零延迟解析 |
| 延迟绑定 | 按需建立连接，减少资源占用 |

---

## 📡 API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/ping` | GET | 健康检查 |
| `/version` | GET | 版本信息 |
| `/config` | GET/PATCH | 配置管理 |
| `/configs` | PUT/PATCH | 配置热重载 |
| `/proxies` | GET | 代理列表 |
| `/proxies/:name` | GET/PUT | 代理详情/切换 |
| `/groups` | GET | 代理组列表 |
| `/rules` | GET | 规则列表 |
| `/connections` | GET/DELETE | 连接管理 |
| `/traffic` | GET | 流量统计 |
| `/dns/query` | GET | DNS 查询 |
| `/logs` | GET | 日志流 (SSE) |
| `/subscriptions` | GET/POST | 订阅管理 |
| `/providers/rules` | GET/PUT | 规则集管理 |
| `/metrics` | GET | Prometheus 指标 |
| `/upgrade` | POST | 自动升级 |

---

## 📝 更新日志

### v1.0.0 (2026-05-04) — 正式版

**核心架构**
- ✨ 新增 `tunnel` 流量调度中枢，统一 TCP/UDP 处理流程
- ✨ 新增 `proxyprovider` 远程代理提供者（订阅解析：YAML/JSON/Base64）
- ✨ 新增 `ConnectionTracker` 连接跟踪器
- ✨ 每代理流量统计 (`ProxyStats`)，Prometheus 指标输出

**协议与传输**
- ✨ 18 种出站协议全覆盖，对齐 mihomo
- ✨ DoH / DoT DNS 解析器
- ✨ 增强型规则集 Provider（MRS 格式支持）
- ✨ Relay 代理组（链式代理）+ 一致性哈希负载均衡

**配置与兼容**
- ✨ Clash YAML 别名支持（`Proxy` / `Proxy Group` / `Rule` 等）
- ✨ 新增 `profile` / `hosts` / `ntp` 配置字段
- ✨ `ParseBytes` / `applyDefaults` 包级别函数
- ✨ GeoSite protobuf 解析器（完整实现）

**基础设施**
- ✨ Prometheus `/metrics` 端点（含每代理指标）
- ✨ SSE 日志流 (`/logs`)
- ✨ 规则集 Provider 自动刷新 (`StartAll`)
- ✨ GeoSite 查询函数注入 (`SetGeoSiteLookup`)
- ✨ 安装脚本安全加固（默认 `allow-lan: false`、绑定 `127.0.0.1`）

### v0.5.0 (2026-04-14) — Windows 平台支持

- 🎉 Windows Service 支持
- 🎉 PowerShell 安装脚本
- 🎉 TUN 模式框架

### v0.4.0 (2026-04-14)

- 🔄 自动升级 API
- 🔧 订阅管理增强
- 📊 性能监控

### v0.3.0 (2026-04-13)

- ✨ RESTful API
- ✨ 健康检查端点
- ✨ 配置热重载

### v0.2.1 (2026-04-14)

- 🐛 修复适配器返回值问题
- ✨ 统一安装脚本 v3.0

### v0.2.0 (2026-04-13)

- ✨ 新增 Hysteria2 / TUIC / WireGuard
- ✨ Clash YAML 完全兼容
- 🔒 TLS 1.3 支持

### v0.1.0 (2026-04-10)

- 🎉 初始版本

---

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建分支：`git checkout -b feature/xxx`
3. 提交更改：`git commit -m 'Add xxx'`
4. 推送：`git push origin feature/xxx`
5. 提交 PR

---

## 📄 许可证

[MIT License](LICENSE)

---

<div align="center">

**Hades** — 高性能代理内核

Made with ❤️ by [Qing060325](https://github.com/Qing060325)

</div>
