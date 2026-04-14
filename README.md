<div align="center">

# 🔱 Hades

**高性能代理内核 — Go 语言编写**

追求极致性能 · 兼容 Clash 配置 · 开箱即用

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Version](https://img.shields.io/github/v/release/Qing060325/Hades?color=blue)](https://github.com/Qing060325/Hades/releases)
[![License](https://img.shields.io/github/license/Qing060325/Hades?color=green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey)]()

[一键安装](#快速开始) · [配置指南](#配置示例) · [Web 面板](https://github.com/Qing060325/Hyperion)

</div>

---

## 📖 简介

**Hades** 是一个使用 Go 语言编写的高性能代理内核，兼容 Clash/Mihomo YAML 配置格式，追求极致性能和简洁体验。

### 🎯 为什么选择 Hades？

| 特性 | 说明 |
|------|------|
| 🚀 **高性能** | Go 编写，零拷贝 I/O，协程池优化 |
| 📦 **开箱即用** | 一键安装脚本，交互式配置向导 |
| 🔄 **Clash 兼容** | 完全兼容 Clash/Mihomo 配置 |
| 🎨 **Web 管理** | 配合 [Hyperion](https://github.com/Qing060325/Hyperion) 面板 |
| 🔒 **安全可靠** | TLS 1.3 / AEAD 加密 / 安全默认配置 |

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
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hades/main/hades_manager.sh | sudo bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/Qing060325/Hades/main/hades_manager.sh | sudo bash
```

安装完成后，管理菜单：
```bash
hades_manager.sh
# 或
sudo hades_manager.sh
```

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
sudo hades_manager.sh

# 2. 安装 Hyperion Web 面板
curl -fsSL https://raw.githubusercontent.com/Qing060325/Hyperion/main/install_hyperion.sh | sudo bash

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
external-controller: 0.0.0.0:9090

# DNS 配置
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  fallback:
    - https://1.1.1.1/dns-query
    - https://8.8.8.8/dns-query

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
  - MATCH,Proxy
```

---

## 🔧 协议支持

| 协议 | 入站 | 出站 | UDP | 特点 |
|------|:--:|:--:|:--:|------|
| HTTP | ✅ | ✅ | ❌ | HTTP CONNECT 代理 |
| SOCKS5 | ✅ | ✅ | ✅ | 标准 SOCKS5 代理 |
| Shadowsocks | ❌ | ✅ | ✅ | AEAD-2022 加密 |
| VMess | ❌ | ✅ | ✅ | V2Ray 协议 |
| VLESS | ❌ | ✅ | ✅ | XTLS-RPRX-Vision |
| Trojan | ❌ | ✅ | ✅ | TLS 伪装 |
| Hysteria2 | ❌ | ✅ | ✅ | 基于 QUIC，抗丢包 |
| TUIC | ❌ | ✅ | ✅ | 基于 QUIC，低延迟 |
| WireGuard | ❌ | ✅ | ✅ | 现代 VPN 协议 |

---

## 🖥️ 命令参考

```bash
# 基础命令
hades -c /etc/hades/config.yaml              # 启动
hades -c /etc/hades/config.yaml -d           # 调试模式
hades -v                                     # 查看版本

# 服务管理（使用 hades_manager.sh）
sudo hades_manager.sh
# 1) 安装/更新 Hades
# 2) 启动服务
# 3) 停止服务
# 4) 重启服务
# 5) 查看实时日志
# 6) 卸载 Hades
# 7) 查看运行状态
```

---

## 🏗️ 项目结构

```
Hades/
├── cmd/
│   └── hades/              # 主程序入口
├── internal/
│   ├── app/                # 应用生命周期
│   ├── config/             # 配置解析
│   └── version/            # 版本信息
├── pkg/
│   ├── api/                # RESTful API
│   ├── core/
│   │   ├── adapter/        # 协议适配器
│   │   ├── dialer/         # 出站拨号
│   │   ├── dns/            # DNS 系统
│   │   ├── group/          # 代理组
│   │   ├── listener/       # 入站监听
│   │   └── rules/          # 规则引擎
│   ├── perf/               # 性能优化
│   └── transport/          # 传输层
├── configs/                # 配置示例
├── docs/                   # 文档
├── hades_manager.sh        # 管理脚本
├── install.sh              # 安装脚本
└── Makefile                # 构建系统
```

---

## 📊 性能优化

| 优化项 | 说明 |
|--------|------|
| 零拷贝 I/O | 减少内存拷贝开销 |
| 协程池 | 高效并发处理 |
| Buffer 池 | 减少 GC 压力 |
| 连接池 | 复用 TCP 连接 |
| 延迟绑定 | 按需建立连接 |

---

## 📝 更新日志

> 完整更新日志请查看 [CHANGELOG.md](CHANGELOG.md)

### v0.5.0 (2026-04-14) - Windows 平台支持

- 🎉 **Windows Service** - 支持 install/uninstall/start/stop/restart/status 命令
- 🎉 **PowerShell 安装脚本** - 一键安装：`(iwr https://raw.githubusercontent.com/Qing060325/Hades/main/install-hades.ps1).Content | iex`
- 🎉 **TUN 模式框架** - 统一的全平台 TUN 设备抽象层
- ✅ 修复 Windows 升级 API 兼容性（文件路径、信号处理）
- ✅ 跨平台编译优化，支持 Windows Service 交叉编译

### v0.4.0 (2026-04-14)

- 🔄 **自动升级 API** - 支持通过 Web 面板一键升级
- 🔧 **订阅管理增强** - 自动更新订阅配置
- 📊 **性能监控** - 添加连接统计和流量监控

### v0.3.0 (2026-04-13)

- ✨ **RESTful API** - 完整的配置管理和运行时 API
- ✨ **健康检查** - `/health` 端点用于服务监控
- ✨ **配置热重载** - 无需重启即可更新配置

### v0.2.1 (2026-04-14)

- 🐛 修复 hysteria2/tuic 适配器 ReadFrom 返回值问题
- 🐛 调整 Go 版本要求从 1.23 降至 1.21
- ✨ `hades_manager.sh` 添加下载超时、文件校验、重试机制
- 📦 提供全平台预编译二进制文件

### v0.2.0 (2026-04-13)

- ✨ 新增 Hysteria2 / TUIC / WireGuard 协议支持
- ✨ 实现 Clash YAML 完全兼容
- ✨ 一键安装脚本与交互式配置向导
- 🔒 TLS 1.3 安全连接支持
- 🔧 API 安全加固

### v0.1.0 (2026-04-10)

- 🎉 初始版本发布
- ✅ 支持 HTTP / SOCKS5 / Shadowsocks / VMess / VLESS / Trojan

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
