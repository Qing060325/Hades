# Hades - 高性能代理内核

<p align="center">
  <img src="https://img.shields.io/badge/version-v0.2.0-blue" alt="version">
  <img src="https://img.shields.io/badge/go-1.21+-00ADD8" alt="go">
  <img src="https://img.shields.io/badge/license-MIT-green" alt="license">
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey" alt="platform">
</p>

---

## 简介

**Hades** 是一个使用 Go 语言编写的高性能代理内核，类似 mihomo/Clash，追求极致性能。

### 核心特性

| 特性 | 描述 |
|------|------|
| 🚀 **高性能** | Linux splice 零拷贝、分级内存池、Goroutine 池 |
| 🔌 **多协议** | SS/VMess/VLESS/Trojan/Hysteria2/TUIC/WireGuard |
| 📦 **开箱即用** | 一键安装、傻瓜式配置、Clash YAML 兼容 |
| 🌐 **TUN 模式** | 跨平台 TUN、gVisor+系统混合栈、DNS 劫持 |
| ⚡ **流量嗅探** | TLS SNI/HTTP Host/QUIC 自动检测 |
| 📊 **RESTful API** | 代理管理/流量统计/连接管理/DNS 查询 |

---

## 快速开始

### 一键安装

```bash
# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/Qing060325/Hades/main/install.sh | bash
```

### 手动安装

```bash
# 克隆仓库
git clone https://github.com/Qing060325/Hades.git
cd Hades

# 构建
make deps
make build

# 运行
./bin/hades -c configs/config.yaml
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

---

## 支持的协议

### 已支持

| 协议 | 入站 | 出站 | UDP | 备注 |
|------|:----:|:----:|:---:|------|
| HTTP | ✅ | ✅ | ❌ | CONNECT 代理 |
| SOCKS5 | ✅ | ✅ | ✅ | SOCKS5 代理 |
| Shadowsocks | ❌ | ✅ | ✅ | AEAD-2022 加密 |
| VMess | ❌ | ✅ | ✅ | WebSocket/gRPC 传输 |
| VLESS | ❌ | ✅ | ✅ | XTLS-RPRX-Vision |
| Trojan | ❌ | ✅ | ✅ | WebSocket/gRPC 传输 |
| Hysteria2 | ❌ | ✅ | ✅ | **新增** - 基于 QUIC |
| TUIC | ❌ | ✅ | ✅ | **新增** - 基于 QUIC |
| WireGuard | ❌ | ✅ | ✅ | **新增** - VPN 协议 |

---

## 配置格式

### Clash YAML 兼容

Hades **完全兼容** Clash/Mihomo 配置格式，可直接使用现有 Clash 配置文件：

```yaml
# 混合端口（HTTP + SOCKS5）
mixed-port: 7890

# 允许局域网
allow-lan: true

# 运行模式
mode: rule

# RESTful API
external-controller: 0.0.0.0:9090

# DNS 配置
dns:
  enable: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query

# 代理节点
proxies:
  - name: "ss-node"
    type: ss
    server: example.com
    port: 443
    cipher: aes-256-gcm
    password: "password"

  - name: "hysteria2-node"
    type: hysteria2
    server: example.com
    port: 443
    password: "password"
    sni: example.com

  - name: "tuic-node"
    type: tuic
    server: example.com
    port: 443
    uuid: "uuid"
    password: "password"

  - name: "wireguard-node"
    type: wireguard
    server: example.com
    port: 51820
    private-key: "private-key"
    public-key: "peer-public-key"

# 代理组
proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - ss-node
      - DIRECT

# 规则
rules:
  - DOMAIN-SUFFIX,google.com,proxy
  - GEOIP,CN,DIRECT
  - MATCH,proxy
```

---

## 协议配置详解

### Hysteria2

基于 QUIC 的高性能代理协议，适合高延迟网络环境。

```yaml
proxies:
  - name: "hysteria2-node"
    type: hysteria2
    server: example.com
    port: 443
    password: "your-password"
    sni: example.com
    skip-cert-verify: false
    # 可选：带宽控制
    up: "100 Mbps"
    down: "100 Mbps"
    # 可选：混淆
    obfs: salamander
    obfs-password: "obfs-password"
```

### TUIC

基于 QUIC 的代理协议，支持多路复用。

```yaml
proxies:
  - name: "tuic-node"
    type: tuic
    server: example.com
    port: 443
    uuid: "your-uuid"
    password: "your-password"
    alpn:
      - h3
    congestion-controller: bbr  # bbr/cubic/new_reno
    sni: example.com
    skip-cert-verify: false
    udp-relay-mode: native  # native/quic
```

### WireGuard

现代化 VPN 协议，高性能加密。

```yaml
proxies:
  - name: "wireguard-node"
    type: wireguard
    server: example.com
    port: 51820
    private-key: "your-private-key"
    public-key: "peer-public-key"
    pre-shared-key: ""
    mtu: 1400
    udp: true
    allowed-ips:
      - "0.0.0.0/0"
      - "::/0"
    reserved: [1, 2, 3]
```

---

## 核心功能

### TUN 模式

```yaml
tun:
  enable: true
  stack: mixed    # system/gvisor/mixed
  dns-hijack:
    - any:53
  auto-route: true
  auto-detect-interface: true
  mtu: 9000
```

### DNS 系统

```yaml
dns:
  enable: true
  enhanced-mode: fake-ip  # fake-ip/redir-host
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - '*.lan'
    - localhost.ptlogin2.qq.com
  nameserver:
    - https://dns.alidns.com/dns-query
  fallback:
    - tls://8.8.4.4
  fallback-filter:
    geoip: true
    geoip-code: CN
```

### 规则引擎

```yaml
rules:
  # 域名规则
  - DOMAIN,example.com,proxy
  - DOMAIN-SUFFIX,google.com,proxy
  - DOMAIN-KEYWORD,github,proxy

  # IP 规则
  - IP-CIDR,192.168.0.0/16,DIRECT
  - GEOIP,CN,DIRECT

  # 进程规则
  - PROCESS-NAME,ssh,DIRECT

  # 最终规则
  - MATCH,proxy
```

### 代理组

```yaml
proxy-groups:
  # 手动选择
  - name: "proxy"
    type: select
    proxies:
      - auto
      - node1
      - DIRECT

  # 自动测速
  - name: "auto"
    type: url-test
    proxies:
      - node1
      - node2
    url: "https://www.gstatic.com/generate_204"
    interval: 300

  # 故障转移
  - name: "fallback"
    type: fallback
    proxies:
      - node1
      - node2
    url: "https://www.gstatic.com/generate_204"
    interval: 300

  # 负载均衡
  - name: "balance"
    type: load-balance
    proxies:
      - node1
      - node2
    strategy: round-robin
```

---

## 性能优化

| 优化项 | 实现方式 |
|--------|----------|
| 零拷贝 | Linux splice 系统调用 |
| 内存池 | 4KB/16KB/32KB/64KB 分级池 |
| 协程池 | IO 密集型 + 计算密集型分离 |
| 连接池 | MaxIdle/MaxActive/IdleTimeout 管理 |
| 缓冲转发 | pool.GetLarge 复用大缓冲区 |

---

## RESTful API

| 端点 | 方法 | 描述 |
|------|------|------|
| `/proxies` | GET | 获取所有代理 |
| `/proxies/:name` | GET | 获取指定代理 |
| `/proxies/:name` | PUT | 切换代理组选择 |
| `/proxies/:name/delay` | GET | 测试延迟 |
| `/rules` | GET | 获取规则列表 |
| `/connections` | GET | 获取连接列表 |
| `/connections/:id` | DELETE | 关闭连接 |
| `/traffic` | GET | 流量统计 (WebSocket) |
| `/logs` | GET | 日志流 (WebSocket) |
| `/dns/query` | GET | DNS 查询 |

---

## 项目结构

```
hades/
├── cmd/hades/              # 程序入口
├── internal/                # 内部模块
│   ├── app/                 # 应用生命周期
│   ├── config/              # 配置管理
│   └── version/             # 版本信息
├── pkg/                     # 核心包
│   ├── core/
│   │   ├── adapter/         # 代理适配器
│   │   ├── listener/        # 入站监听器
│   │   ├── rules/           # 规则引擎
│   │   ├── dns/             # DNS 系统
│   │   └── group/           # 代理组
│   ├── transport/           # 传输层
│   ├── crypto/              # 加密库
│   ├── perf/                # 性能优化
│   └── api/                 # API 接口
└── configs/                 # 配置文件
```

---

## 常用命令

```bash
# 启动
hades -c /etc/hades/config.yaml

# 调试模式
hades -c /etc/hades/config.yaml -d

# 查看版本
hades -v

# 服务管理
hades-ctl start    # 启动
hades-ctl stop     # 停止
hades-ctl restart  # 重启
hades-ctl status   # 状态
hades-ctl logs     # 日志
```

---

## 构建

```bash
# 安装依赖
make deps

# 构建当前平台
make build

# 跨平台编译
make cross-compile

# 运行测试
make test

# 性能测试
make bench
```

---

## 鸣谢

参考项目：
- [mihomo](https://github.com/MetaCubeX/mihomo)
- [clash](https://github.com/Dreamacro/clash)

---

## 许可证

[MIT License](LICENSE)

---

## 贡献

欢迎提交 Issue 和 Pull Request！
