# Hades 高性能代理内核部署报告

## 项目概述

**Hades** 是一个使用 Go 语言编写的高性能代理内核，类似于 mihomo/Clash，追求极致性能。

## 部署信息

| 项目 | 详情 |
|------|------|
| 源码地址 | https://github.com/Qing060325/Hades |
| 部署路径 | `/workspace/Hades` |
| 可执行文件 | `/workspace/Hades/bin/hades` |
| 版本 | v0.5.0 |
| 构建时间 | 2026-05-04 |

## 服务端口

| 服务 | 端口 | 绑定地址 | 说明 |
|------|------|----------|------|
| 混合端口 | 7890 | 0.0.0.0 | HTTP + SOCKS5 自动协议检测 |
| API 服务 | 9090 | 127.0.0.1 | RESTful API 管理接口 |
| DNS 服务 | 1053 | 127.0.0.1 | DNS 解析服务 |

## 使用方法

### 快速启动

```bash
# 方式1: 使用安装脚本（推荐）
bash install.sh install    # 安装
bash install.sh start      # 启动
bash install.sh stop       # 停止
bash install.sh restart    # 重启
bash install.sh status     # 状态
bash install.sh logs       # 日志

# 方式2: 直接运行
/workspace/Hades/bin/hades -c /etc/hades/config.yaml
```

> ⚠️ `hades.sh` 已废弃，请使用 `install.sh` 代替。

### 命令行参数

| 参数 | 说明 |
|------|------|
| `-c <path>` | 指定配置文件路径 |
| `-d` | 启用调试模式 |
| `-v` | 显示版本信息 |

## 配置文件

| 文件 | 说明 |
|------|------|
| `configs/config.yaml` | 完整配置示例 |
| `configs/test-config.yaml` | 最小化测试配置 |

### 主要配置项

```yaml
# 混合端口（HTTP + SOCKS5）
mixed-port: 7890

# 运行模式: rule / global / direct
mode: rule

# RESTful API（仅本地访问）
external-controller: 127.0.0.1:9090

# DNS 配置
dns:
  enable: true
  enhanced-mode: fake-ip

# TUN 模式（可选）
tun:
  enable: false
  stack: mixed
```

## 支持的协议

### ✅ 已支持
- HTTP/HTTPS (入站 + 出站)
- SOCKS5 (入站 + 出站)
- Shadowsocks (AEAD-2022)
- VMess (WebSocket传输)
- VLESS (XTLS-RPRX-Vision)
- Trojan (TLS/WebSocket/gRPC)

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

## 性能优化

- Linux splice 零拷贝
- 分级内存池 (4KB/16KB/32KB/64KB)
- Goroutine 池
- 连接复用

## 验证测试

```
✅ 依赖安装成功
✅ 项目构建成功
✅ 服务启动测试通过
✅ DNS 服务正常 (端口 1053)
✅ 混合端口正常 (端口 7890)
✅ API 服务正常 (端口 9090)
```

## 下一步

1. 根据需要修改 `configs/config.yaml` 配置文件
2. 添加实际的代理节点
3. 配置分流规则
4. 启动服务: `./hades.sh start`
