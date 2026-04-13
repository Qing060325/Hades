# Hades 协议说明

本文档详细介绍 Hades 支持的所有代理协议。

---

## 协议对比

| 协议 | 传输层 | 加密 | UDP | 多路复用 | 零拷贝 | 推荐场景 |
|------|--------|------|:---:|:--------:|:------:|----------|
| HTTP | TCP | TLS | ❌ | ❌ | ❌ | 简单代理 |
| SOCKS5 | TCP | TLS | ✅ | ❌ | ❌ | 通用代理 |
| Shadowsocks | TCP/UDP | AEAD | ✅ | ❌ | ❌ | 传统代理 |
| VMess | TCP/WS/gRPC | AEAD | ✅ | ❌ | ❌ | V2Ray 生态 |
| VLESS | TCP/WS/gRPC | TLS | ✅ | ❌ | ✅ | 高性能代理 |
| Trojan | TCP/WS/gRPC | TLS | ✅ | ❌ | ❌ | HTTPS 伪装 |
| Hysteria2 | QUIC (UDP) | TLS | ✅ | ✅ | ❌ | 高延迟网络 |
| TUIC | QUIC (UDP) | TLS | ✅ | ✅ | ❌ | 低延迟优化 |
| WireGuard | UDP | ChaCha20 | ✅ | ✅ | ✅ | VPN 场景 |

---

## HTTP 代理

### 协议简介

HTTP 代理是最常见的代理协议，通过 HTTP CONNECT 方法建立隧道。

### 工作原理

```
客户端 → [HTTP CONNECT] → 代理服务器 → 目标服务器
```

### 配置示例

```yaml
proxies:
  - name: "http-proxy"
    type: http
    server: proxy.example.com
    port: 8080
    username: "user"        # 可选
    password: "pass"        # 可选
    tls: true               # HTTPS 代理
    sni: proxy.example.com
```

### 特点

- ✅ 广泛支持
- ✅ 简单易用
- ❌ 不原生支持 UDP
- ❌ 无多路复用

---

## SOCKS5 代理

### 协议简介

SOCKS5 是一种网络传输协议，支持 TCP 和 UDP。

### 工作原理

```
客户端 → [SOCKS5 握手] → [认证] → [连接请求] → 目标服务器
```

### 配置示例

```yaml
proxies:
  - name: "socks5-proxy"
    type: socks5
    server: proxy.example.com
    port: 1080
    username: "user"
    password: "pass"
    tls: true               # SOCKS5 over TLS
    udp: true               # 启用 UDP
```

### 特点

- ✅ 支持 TCP 和 UDP
- ✅ 支持认证
- ✅ 广泛支持
- ❌ 无加密 (需配合 TLS)

---

## Shadowsocks

### 协议简介

Shadowsocks 是一种轻量级代理协议，使用 AEAD 加密。

### 加密方式

| 加密算法 | 密钥长度 | 推荐度 |
|----------|----------|:------:|
| aes-128-gcm | 128 bit | ⭐⭐⭐ |
| aes-256-gcm | 256 bit | ⭐⭐⭐⭐ |
| chacha20-ietf-poly1305 | 256 bit | ⭐⭐⭐⭐ |
| 2022-blake3-aes-128-gcm | 128 bit | ⭐⭐⭐⭐⭐ |
| 2022-blake3-aes-256-gcm | 256 bit | ⭐⭐⭐⭐⭐ |

### 配置示例

```yaml
proxies:
  - name: "ss-node"
    type: ss
    server: ss.example.com
    port: 443
    cipher: aes-256-gcm
    password: "your-password"
    udp: true

    # 插件
    plugin: obfs
    plugin-opts:
      mode: tls
      host: www.example.com
```

### 特点

- ✅ 轻量高效
- ✅ 支持插件
- ✅ AEAD 加密
- ⚠️ 特征明显

---

## VMess

### 协议简介

VMess 是 V2Ray 原创的代理协议，支持多种传输方式。

### 传输方式

| 传输 | 描述 |
|------|------|
| tcp | 原生 TCP |
| ws | WebSocket |
| grpc | gRPC (HTTP/2) |

### 配置示例

```yaml
proxies:
  - name: "vmess-node"
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: "your-uuid"
    alter-id: 0
    cipher: auto

    # TLS
    tls: true
    sni: vmess.example.com

    # WebSocket
    network: ws
    ws-path: /vmess
    ws-headers:
      Host: vmess.example.com
```

### 特点

- ✅ 自研加密
- ✅ 多种传输
- ✅ 支持 AEAD
- ⚠️ alterID 已弃用

---

## VLESS

### 协议简介

VLESS 是 VMess 的简化版，去除冗余加密，依赖 TLS 保护。

### XTLS Vision

VLESS 支持 XTLS Vision 流控，可实现接近原生的性能。

### 配置示例

```yaml
proxies:
  - name: "vless-node"
    type: vless
    server: vless.example.com
    port: 443
    uuid: "your-uuid"

    # TLS
    tls: true
    sni: vless.example.com

    # XTLS Vision (高性能)
    flow: xtls-rprx-vision

    # 或使用 WebSocket
    # network: ws
    # ws-path: /vless
```

### 特点

- ✅ 高性能 (XTLS Vision)
- ✅ 简洁协议
- ✅ 多种传输
- ⚠️ 必须配合 TLS

---

## Trojan

### 协议简介

Trojan 伪装成 HTTPS 流量，使用真实 TLS 证书。

### 协议格式

```
SHA224(password) + CRLF + command + target_addr + CRLF + payload
```

### 配置示例

```yaml
proxies:
  - name: "trojan-node"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "your-password"
    udp: true
    sni: trojan.example.com

    # WebSocket
    network: ws
    ws-path: /trojan
```

### 特点

- ✅ HTTPS 伪装
- ✅ 支持多传输
- ✅ 简单配置
- ⚠️ 需要证书

---

## Hysteria2

### 协议简介

Hysteria2 是基于 QUIC 协议的高性能代理协议，专为高延迟网络优化。

### 技术特点

- **QUIC 传输**: 基于 UDP，支持 0-RTT 连接
- **多路复用**: 单连接多流
- **拥塞控制**: 自适应带宽
- **混淆**: Salamander 混淆

### 配置示例

```yaml
proxies:
  - name: "hysteria2-node"
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: "your-password"
    sni: hy2.example.com

    # 带宽设置
    up: "100 Mbps"
    down: "100 Mbps"

    # 混淆
    obfs: salamander
    obfs-password: "obfs-password"
```

### 带宽设置

| 设置 | 说明 |
|------|------|
| `up` | 上行带宽 |
| `down` | 下行带宽 |

支持格式:
- `100 Mbps`
- `1 Gbps`
- `10000000` (bps)

### 特点

- ✅ 高延迟优化
- ✅ 多路复用
- ✅ 内置 UDP
- ✅ 0-RTT 连接
- ⚠️ 基于 UDP，可能被 QoS

---

## TUIC

### 协议简介

TUIC 是基于 QUIC 的代理协议，注重低延迟和高性能。

### 技术特点

- **QUIC 传输**: UDP 原生支持
- **零拥塞控制**: 可禁用内置拥塞控制
- **UDP 中继**: native 或 quic 模式
- **多路复用**: 高效流管理

### 配置示例

```yaml
proxies:
  - name: "tuic-node"
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: "your-uuid"
    password: "your-password"

    # ALPN
    alpn:
      - h3

    # 拥塞控制
    congestion-controller: bbr

    # UDP 中继模式
    udp-relay-mode: native

    # 心跳
    heartbeat-interval: 10000
```

### 拥塞控制

| 算法 | 说明 |
|------|------|
| `bbr` | Google BBR (推荐) |
| `cubic` | 标准 TCP 拥塞控制 |
| `new_reno` | 经典 TCP 拥塞控制 |

### UDP 中继模式

| 模式 | 说明 |
|------|------|
| `native` | 原生 UDP 转发 |
| `quic` | QUIC 流封装 |

### 特点

- ✅ 低延迟
- ✅ UDP 优化
- ✅ 多路复用
- ⚠️ 基于 UDP

---

## WireGuard

### 协议简介

WireGuard 是现代 VPN 协议，以简洁高效著称。

### 技术特点

- **曲线加密**: Curve25519 密钥交换
- **对称加密**: ChaCha20-Poly1305
- **哈希**: BLAKE2s
- **协议**: 无状态，简洁

### 配置示例

```yaml
proxies:
  - name: "wireguard-node"
    type: wireguard
    server: wg.example.com
    port: 51820
    private-key: "your-private-key"
    public-key: "peer-public-key"
    pre-shared-key: ""        # 可选
    mtu: 1400
    udp: true
    reserved: [1, 2, 3]

    # 允许的 IP
    allowed-ips:
      - "0.0.0.0/0"
      - "::/0"
```

### 密钥生成

```bash
# 生成私钥
wg genkey > privatekey

# 生成公钥
wg pubkey < privatekey > publickey

# 生成预共享密钥 (可选)
wg genpsk > presharedkey
```

### 特点

- ✅ 极简协议
- ✅ 高性能加密
- ✅ 现代 VPN
- ✅ 审计安全
- ⚠️ 特征明显

---

## 协议选择建议

### 按场景选择

| 场景 | 推荐协议 | 原因 |
|------|----------|------|
| 日常使用 | VLESS + Vision | 高性能，低特征 |
| 高延迟网络 | Hysteria2 | QUIC 优化 |
| 企业防火墙 | Trojan | HTTPS 伪装 |
| 移动网络 | TUIC | 低延迟优化 |
| VPN 需求 | WireGuard | 现代 VPN |
| 兼容性 | VMess | 广泛支持 |
| 传统环境 | Shadowsocks | 简单稳定 |

### 按需求选择

| 需求 | 推荐协议 |
|------|----------|
| 最高性能 | VLESS + Vision |
| 最佳 UDP | Hysteria2 / TUIC |
| 最强伪装 | Trojan |
| 最简配置 | Shadowsocks |
| VPN 功能 | WireGuard |

---

## 安全建议

1. **始终启用 TLS**: VLESS/Trojan 必须使用 TLS
2. **使用强加密**: 选择 AEAD 加密算法
3. **定期更新密钥**: 定期更换密码和密钥
4. **启用证书验证**: 除非必要，不要跳过证书验证
5. **使用混淆**: 在特征明显的环境中启用混淆
