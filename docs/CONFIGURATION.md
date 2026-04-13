# Hades 配置指南

本文档详细介绍 Hades 的所有配置选项。

---

## 配置文件位置

Hades 按以下顺序查找配置文件：

1. 命令行指定: `hades -c /path/to/config.yaml`
2. 环境变量: `$HADES_CONFIG`
3. 默认位置: `/etc/hades/config.yaml`
4. 当前目录: `./config.yaml`

---

## 基础配置

### 端口配置

```yaml
# HTTP 代理端口
port: 7890

# SOCKS5 代理端口
socks-port: 7891

# 混合端口 (HTTP + SOCKS5 自动检测)
mixed-port: 7890

# 透明代理端口 (Linux)
redir-port: 7892
tproxy-port: 7893
```

### 网络配置

```yaml
# 允许局域网连接
allow-lan: true

# 绑定地址
bind-address: "*"

# IPv6 支持
ipv6: false
```

### 运行模式

```yaml
# 运行模式: rule / global / direct
mode: rule

# 日志级别: silent / error / warning / info / debug
log-level: info
```

### 外部控制

```yaml
# RESTful API 监听地址
external-controller: 0.0.0.0:9090

# Web UI 目录
external-ui: /path/to/ui

# API 密钥
secret: "your-secret"
```

---

## 代理节点配置

### HTTP 代理

```yaml
proxies:
  - name: "http-proxy"
    type: http
    server: example.com
    port: 8080
    username: "user"        # 可选
    password: "pass"        # 可选
    tls: true               # 可选
    sni: example.com        # 可选
    skip-cert-verify: false # 可选
```

### SOCKS5 代理

```yaml
proxies:
  - name: "socks5-proxy"
    type: socks5
    server: example.com
    port: 1080
    username: "user"        # 可选
    password: "pass"        # 可选
    tls: true               # 可选
    sni: example.com        # 可选
    udp: true               # 可选
```

### Shadowsocks

```yaml
proxies:
  - name: "ss-node"
    type: ss
    server: example.com
    port: 443
    cipher: aes-256-gcm     # 加密方式
    password: "password"
    udp: true               # 可选

    # 插件 (可选)
    plugin: obfs
    plugin-opts:
      mode: tls
      host: example.com
```

**支持的加密方式:**
- `aes-128-gcm`
- `aes-256-gcm`
- `chacha20-ietf-poly1305`
- `2022-blake3-aes-128-gcm`
- `2022-blake3-aes-256-gcm`

### VMess

```yaml
proxies:
  - name: "vmess-node"
    type: vmess
    server: example.com
    port: 443
    uuid: "your-uuid"
    alter-id: 0
    cipher: auto
    udp: true

    # TLS
    tls: true
    sni: example.com
    skip-cert-verify: false

    # WebSocket 传输
    network: ws
    ws-path: /vmess
    ws-headers:
      Host: example.com

    # gRPC 传输
    # network: grpc
    # grpc-service-name: vmess
```

### VLESS

```yaml
proxies:
  - name: "vless-node"
    type: vless
    server: example.com
    port: 443
    uuid: "your-uuid"
    udp: true

    # TLS
    tls: true
    sni: example.com
    skip-cert-verify: false

    # XTLS Vision
    flow: xtls-rprx-vision

    # WebSocket 传输
    network: ws
    ws-path: /vless

    # gRPC 传输
    # network: grpc
    # grpc-service-name: vless

    # Reality (可选)
    # reality-opts:
    #   public-key: xxx
    #   short-id: xxx
```

### Trojan

```yaml
proxies:
  - name: "trojan-node"
    type: trojan
    server: example.com
    port: 443
    password: "your-password"
    udp: true
    sni: example.com
    skip-cert-verify: false

    # WebSocket 传输
    network: ws
    ws-path: /trojan

    # gRPC 传输
    # network: grpc
    # grpc-service-name: trojan
```

### Hysteria2

```yaml
proxies:
  - name: "hysteria2-node"
    type: hysteria2
    server: example.com
    port: 443
    password: "your-password"
    sni: example.com
    skip-cert-verify: false

    # 带宽控制 (可选)
    up: "100 Mbps"
    down: "100 Mbps"

    # 混淆 (可选)
    obfs: salamander
    obfs-password: "obfs-password"
```

**参数说明:**

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| server | string | ✅ | 服务器地址 |
| port | int | ✅ | 服务器端口 |
| password | string | ✅ | 认证密码 |
| sni | string | ❌ | TLS SNI |
| skip-cert-verify | bool | ❌ | 跳过证书验证 |
| up | string | ❌ | 上行带宽限制 |
| down | string | ❌ | 下行带宽限制 |
| obfs | string | ❌ | 混淆类型 (salamander) |
| obfs-password | string | ❌ | 混淆密码 |

### TUIC

```yaml
proxies:
  - name: "tuic-node"
    type: tuic
    server: example.com
    port: 443
    uuid: "your-uuid"
    password: "your-password"
    udp: true

    # ALPN
    alpn:
      - h3

    # 拥塞控制
    congestion-controller: bbr  # bbr / cubic / new_reno

    # SNI
    sni: example.com
    skip-cert-verify: false

    # UDP 中继模式
    udp-relay-mode: native  # native / quic

    # 心跳间隔
    heartbeat-interval: 10000  # ms
```

**参数说明:**

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| server | string | ✅ | 服务器地址 |
| port | int | ✅ | 服务器端口 |
| uuid | string | ✅ | 用户 UUID |
| password | string | ✅ | 认证密码 |
| alpn | []string | ❌ | ALPN 协议列表 |
| congestion-controller | string | ❌ | 拥塞控制算法 |
| udp-relay-mode | string | ❌ | UDP 中继模式 |
| heartbeat-interval | int | ❌ | 心跳间隔 (ms) |

### WireGuard

```yaml
proxies:
  - name: "wireguard-node"
    type: wireguard
    server: example.com
    port: 51820
    private-key: "your-private-key"
    public-key: "peer-public-key"
    pre-shared-key: ""        # 可选
    mtu: 1400                 # 可选
    udp: true                 # 可选
    reserved: [1, 2, 3]       # 可选

    # 允许的 IP 范围
    allowed-ips:
      - "0.0.0.0/0"
      - "::/0"
```

**参数说明:**

| 参数 | 类型 | 必填 | 说明 |
|------|------|:----:|------|
| server | string | ✅ | 服务器地址 |
| port | int | ✅ | 服务器端口 |
| private-key | string | ✅ | 本地私钥 |
| public-key | string | ✅ | 对端公钥 |
| pre-shared-key | string | ❌ | 预共享密钥 |
| mtu | int | ❌ | MTU 值 (默认 1400) |
| reserved | []int | ❌ | WireGuard reserved 字段 |
| allowed-ips | []string | ❌ | 允许的 IP 范围 |

---

## 代理组配置

### Select (手动选择)

```yaml
proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - auto
      - node1
      - node2
      - DIRECT
```

### URLTest (自动测速)

```yaml
proxy-groups:
  - name: "auto"
    type: url-test
    proxies:
      - node1
      - node2
    url: "https://www.gstatic.com/generate_204"
    interval: 300        # 测速间隔 (秒)
    tolerance: 50        # 容差 (ms)
    timeout: 5000        # 超时 (ms)
    lazy: true           # 懒加载
```

### Fallback (故障转移)

```yaml
proxy-groups:
  - name: "fallback"
    type: fallback
    proxies:
      - node1
      - node2
    url: "https://www.gstatic.com/generate_204"
    interval: 300
    timeout: 5000
```

### LoadBalance (负载均衡)

```yaml
proxy-groups:
  - name: "balance"
    type: load-balance
    proxies:
      - node1
      - node2
    strategy: round-robin  # round-robin / consistent-hashing
    url: "https://www.gstatic.com/generate_204"
    interval: 300
```

---

## 规则配置

### 域名规则

```yaml
rules:
  # 精确匹配
  - DOMAIN,example.com,proxy

  # 后缀匹配
  - DOMAIN-SUFFIX,google.com,proxy

  # 关键字匹配
  - DOMAIN-KEYWORD,github,proxy

  # 正则匹配
  - DOMAIN-REGEX,^.*\.google\.com$,proxy
```

### IP 规则

```yaml
rules:
  # CIDR 匹配
  - IP-CIDR,192.168.0.0/16,DIRECT
  - IP-CIDR6,2001:db8::/32,DIRECT

  # GeoIP 匹配
  - GEOIP,CN,DIRECT
  - GEOIP,US,proxy

  # IPSET 匹配 (Linux)
  - IPSET,chnroute,DIRECT
```

### 端口规则

```yaml
rules:
  # 端口匹配
  - DST-PORT,80,proxy
  - DST-PORT,443,proxy

  # 端口范围
  - DST-PORT,1000-2000,DIRECT
```

### 进程规则

```yaml
rules:
  # 进程名匹配
  - PROCESS-NAME,ssh,DIRECT
  - PROCESS-NAME,curl,proxy

  # 进程路径匹配
  - PROCESS-PATH,/usr/bin/ssh,DIRECT
```

### 规则集

```yaml
rule-providers:
  reject:
    type: http
    behavior: domain
    url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/reject.txt"
    path: ./ruleset/reject.yaml
    interval: 86400

  proxy:
    type: http
    behavior: domain
    url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/proxy.txt"
    path: ./ruleset/proxy.yaml
    interval: 86400

rules:
  - RULE-SET,reject,REJECT
  - RULE-SET,proxy,proxy
  - MATCH,DIRECT
```

---

## DNS 配置

### Fake-IP 模式

```yaml
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16

  # Fake-IP 过滤
  fake-ip-filter:
    - '*.lan'
    - localhost.ptlogin2.qq.com
    - '+.srv.nintendo.net'

  # 基础 DNS
  default-nameserver:
    - 223.5.5.5
    - 119.29.29.29

  # 主 DNS
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query

  # 回退 DNS
  fallback:
    - tls://8.8.4.4
    - tls://1.1.1.1

  # 回退过滤
  fallback-filter:
    geoip: true
    geoip-code: CN
```

### Redir-Host 模式

```yaml
dns:
  enable: true
  enhanced-mode: redir-host

  nameserver:
    - https://dns.alidns.com/dns-query

  fallback:
    - tls://8.8.4.4
```

---

## TUN 模式配置

```yaml
tun:
  enable: true

  # 协议栈: system / gvisor / mixed
  stack: mixed

  # DNS 劫持
  dns-hijack:
    - any:53

  # 自动路由
  auto-route: true

  # 自动检测接口
  auto-detect-interface: true

  # 严格路由
  strict-route: false

  # MTU
  mtu: 9000
```

---

## 完整配置示例

```yaml
# 基础配置
mixed-port: 7890
allow-lan: true
mode: rule
log-level: info

# 外部控制
external-controller: 0.0.0.0:9090
secret: ""

# DNS
dns:
  enable: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query

# TUN
tun:
  enable: false
  stack: mixed

# 代理节点
proxies:
  - name: "node1"
    type: ss
    server: example.com
    port: 443
    cipher: aes-256-gcm
    password: "password"

# 代理组
proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - node1
      - DIRECT

# 规则
rules:
  - GEOIP,CN,DIRECT
  - MATCH,proxy
```

---

## 环境变量

| 变量 | 说明 |
|------|------|
| `HADES_CONFIG` | 配置文件路径 |
| `HADES_SECRET` | API 密钥 |
| `HADES_LOG_LEVEL` | 日志级别 |
