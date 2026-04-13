# Hades

**Hades 高性能代理内核** - 使用 Go 语言编写的类 mihomo/Clash 代理内核，追求极致性能。

## 特性

### 协议支持
- ✅ HTTP/HTTPS 代理（入站+出站）
- ✅ SOCKS5 代理（入站+出站）
- ✅ Shadowsocks (AEAD-2022 / aes-256-gcm / chacha20-poly1305)
- ✅ VMess (AEAD加密 / WebSocket传输)
- ✅ VLESS + XTLS-RPRX-Vision
- ✅ Trojan (TLS伪装 / WebSocket/gRPC传输)
- 🔲 Hysteria / Hysteria2（框架就绪）
- 🔲 TUIC（框架就绪）
- 🔲 WireGuard（框架就绪）
- 🔲 Snell（框架就绪）
- 🔲 SSH（框架就绪）

### 核心功能
- ✅ 混合端口监听 (HTTP + SOCKS5 自动协议检测)
- ✅ TUN 模式 (跨平台 / gVisor+系统混合栈 / DNS劫持)
- ✅ 规则引擎 (DOMAIN/DOMAIN-SUFFIX/DOMAIN-KEYWORD/IPCIDR/GEOIP/MATCH)
- ✅ DNS 系统 (Fake-IP池 / LRU缓存 / DoH/DoT)
- ✅ 代理组 (Select/URLTest/Fallback/LoadBalance/Round-Robin)
- ✅ RESTful API (代理管理/流量统计/连接管理/DNS查询/日志流)
- ✅ 流量嗅探 (TLS SNI/HTTP Host/QUIC 自动检测)
- ✅ WebSocket 传输层
- ✅ gRPC 传输层
- ✅ 连接池 (复用后端连接/自动回收/超时管理)
- ✅ 流量统计 (上传/下载/速度/格式化)

### 性能优化
- ✅ Linux splice 零拷贝 (pipe pool复用)
- ✅ 跨平台零拷贝抽象 (Linux splice / 通用缓冲池自动切换)
- ✅ 分级内存池 (4KB/16KB/32KB/64KB, sync.Pool自动GC)
- ✅ Goroutine 池 (IO密集型+计算密集型分离)
- ✅ 连接池 (MaxIdle/MaxActive/IdleTimeout/MaxLifetime)
- ✅ 缓冲双向转发 (pool.GetLarge 复用)

## 快速开始

### 构建

```bash
# 安装依赖
make deps

# 构建
make build

# 跨平台编译
make cross-compile
```

### 运行

```bash
# 使用配置文件运行
./bin/hades -c configs/config.yaml

# 调试模式
./bin/hades -c configs/config.yaml -d

# 查看版本
./bin/hades -v
```

## 配置说明

配置文件采用 YAML 格式，完整示例见 `configs/config.yaml`。

### 基础配置

```yaml
# 混合端口（HTTP + SOCKS5）
mixed-port: 7890

# 允许局域网连接
allow-lan: true

# 运行模式: rule / global / direct
mode: rule

# 日志级别
log-level: info
```

### TUN 模式

```yaml
tun:
  enable: true
  stack: mixed    # system / gvisor / mixed
  dns-hijack:
    - any:53
  auto-route: true
```

### DNS 配置

```yaml
dns:
  enable: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query
```

### 代理节点

```yaml
proxies:
  - name: "ss-node"
    type: ss
    server: example.com
    port: 443
    cipher: aes-256-gcm
    password: "password"
```

### 代理组

```yaml
proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - node1
      - DIRECT

  - name: "auto"
    type: url-test
    proxies:
      - node1
      - node2
    url: "https://www.gstatic.com/generate_204"
    interval: 300
```

### 规则

```yaml
rules:
  - DOMAIN-SUFFIX,google.com,proxy
  - GEOIP,CN,DIRECT
  - MATCH,proxy
```

## 性能基准

| 指标 | 目标值 |
|------|--------|
| TCP 转发吞吐量 | > 10 Gbps |
| TCP 转发延迟 | < 1ms |
| UDP 转发吞吐量 | > 5 Gbps |
| 内存占用 | < 50MB (空载) |
| 连接处理能力 | > 100K 并发 |

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
│   │   ├── dns/             # DNS系统
│   │   └── group/           # 代理组
│   ├── transport/           # 传输层
│   ├── crypto/              # 加密库
│   ├── perf/                # 性能优化
│   └── api/                 # API接口
└── configs/                 # 配置文件
```

## 开发

### 运行测试

```bash
make test
```

### 性能测试

```bash
make bench
```

### 代码检查

```bash
make lint
```

## 许可证

MIT License

## 致谢

本项目参考了以下开源项目的设计思路：
- [mihomo](https://github.com/MetaCubeX/mihomo)
- [clash](https://github.com/Dreamacro/clash)
