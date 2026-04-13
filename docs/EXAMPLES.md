# Hades 配置示例

本文档提供各种场景的完整配置示例。

---

## 目录

1. [最小化配置](#最小化配置)
2. [完整配置](#完整配置)
3. [Clash 兼容配置](#clash-兼容配置)
4. [多协议配置](#多协议配置)
5. [企业级配置](#企业级配置)
6. [透明代理配置](#透明代理配置)

---

## 最小化配置

最简单的配置，适合快速启动。

```yaml
# 最小化配置 - 开箱即用

# 混合端口
mixed-port: 7890

# 允许局域网
allow-lan: true

# 运行模式
mode: rule

# 日志级别
log-level: info

# DNS
dns:
  enable: true
  enhanced-mode: fake-ip
  nameserver:
    - https://dns.alidns.com/dns-query

# 代理组
proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - DIRECT

# 规则
rules:
  - GEOIP,CN,DIRECT
  - MATCH,DIRECT
```

---

## 完整配置

包含所有功能模块的完整配置。

```yaml
# Hades 完整配置示例

# ========== 基础配置 ==========
mixed-port: 7890
port: 0                  # HTTP 端口 (0 表示禁用)
socks-port: 0            # SOCKS5 端口 (0 表示禁用)
allow-lan: true
bind-address: "*"
mode: rule
log-level: info
ipv6: false

# ========== 外部控制 ==========
external-controller: 0.0.0.0:9090
external-ui: /path/to/ui
secret: ""

# ========== TUN 模式 ==========
tun:
  enable: false
  stack: mixed
  dns-hijack:
    - any:53
  auto-route: true
  auto-detect-interface: true
  strict-route: false
  mtu: 9000

# ========== DNS 配置 ==========
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16

  fake-ip-filter:
    - '*.lan'
    - localhost.ptlogin2.qq.com
    - '+.srv.nintendo.net'
    - '+.stun.playstation.net'

  default-nameserver:
    - 223.5.5.5
    - 119.29.29.29

  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query

  fallback:
    - tls://8.8.4.4
    - tls://1.1.1.1

  fallback-filter:
    geoip: true
    geoip-code: CN

# ========== 嗅探配置 ==========
sniffer:
  enable: true
  override-destination: true
  force-dns-mapping: true
  parse-pure-ip: true
  sniff:
    HTTP:
      ports: [80, 8080-8880]
    TLS:
      ports: [443, 8443]
    QUIC:
      ports: [443, 8443]

# ========== 代理节点 ==========
proxies:
  # Shadowsocks
  - name: "ss-node-1"
    type: ss
    server: ss1.example.com
    port: 443
    cipher: aes-256-gcm
    password: "password123"
    udp: true

  - name: "ss-node-2"
    type: ss
    server: ss2.example.com
    port: 443
    cipher: 2022-blake3-aes-256-gcm
    password: "password456"
    udp: true

  # VMess
  - name: "vmess-node"
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: "your-uuid-here"
    alter-id: 0
    cipher: auto
    tls: true
    sni: vmess.example.com
    network: ws
    ws-path: /vmess
    ws-headers:
      Host: vmess.example.com

  # VLESS + Vision
  - name: "vless-vision"
    type: vless
    server: vless.example.com
    port: 443
    uuid: "your-uuid-here"
    tls: true
    sni: vless.example.com
    flow: xtls-rprx-vision

  # VLESS + WebSocket
  - name: "vless-ws"
    type: vless
    server: vless-ws.example.com
    port: 443
    uuid: "your-uuid-here"
    tls: true
    sni: vless-ws.example.com
    network: ws
    ws-path: /vless

  # Trojan
  - name: "trojan-node"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "your-password"
    udp: true
    sni: trojan.example.com

  # Trojan + gRPC
  - name: "trojan-grpc"
    type: trojan
    server: trojan-grpc.example.com
    port: 443
    password: "your-password"
    sni: trojan-grpc.example.com
    network: grpc
    grpc-service-name: trojan

  # Hysteria2
  - name: "hysteria2-node"
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: "your-password"
    sni: hy2.example.com
    up: "100 Mbps"
    down: "100 Mbps"

  # TUIC
  - name: "tuic-node"
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: "your-uuid-here"
    password: "your-password"
    alpn:
      - h3
    congestion-controller: bbr
    sni: tuic.example.com
    udp-relay-mode: native

  # WireGuard
  - name: "wireguard-node"
    type: wireguard
    server: wg.example.com
    port: 51820
    private-key: "your-private-key"
    public-key: "peer-public-key"
    mtu: 1400
    udp: true
    allowed-ips:
      - "0.0.0.0/0"
      - "::/0"

# ========== 代理组 ==========
proxy-groups:
  # 主选择组
  - name: "proxy"
    type: select
    proxies:
      - auto-select
      - fallback-group
      - ss-node-1
      - vmess-node
      - vless-vision
      - trojan-node
      - DIRECT

  # 自动选择
  - name: "auto-select"
    type: url-test
    proxies:
      - ss-node-1
      - ss-node-2
      - vmess-node
      - vless-vision
      - trojan-node
    url: "https://www.gstatic.com/generate_204"
    interval: 300
    tolerance: 50
    timeout: 5000

  # 故障转移
  - name: "fallback-group"
    type: fallback
    proxies:
      - vless-vision
      - trojan-node
      - vmess-node
      - ss-node-1
    url: "https://www.gstatic.com/generate_204"
    interval: 300

  # 负载均衡
  - name: "load-balance"
    type: load-balance
    proxies:
      - ss-node-1
      - ss-node-2
      - trojan-node
    strategy: round-robin

  # 流媒体
  - name: "streaming"
    type: select
    proxies:
      - proxy
      - auto-select
      - DIRECT

  # 国内直连
  - name: "domestic"
    type: select
    proxies:
      - DIRECT
      - proxy

# ========== 规则 ==========
rules:
  # 流媒体
  - DOMAIN-SUFFIX,netflix.com,streaming
  - DOMAIN-SUFFIX,nflxvideo.net,streaming
  - DOMAIN-SUFFIX,youtube.com,streaming
  - DOMAIN-SUFFIX,googlevideo.com,streaming

  # 国内直连
  - DOMAIN-SUFFIX,cn,domestic
  - DOMAIN-KEYWORD,baidu,domestic
  - DOMAIN-KEYWORD,taobao,domestic
  - DOMAIN-KEYWORD,alipay,domestic

  # 代理
  - DOMAIN-SUFFIX,google.com,proxy
  - DOMAIN-SUFFIX,googleapis.com,proxy
  - DOMAIN-SUFFIX,github.com,proxy
  - DOMAIN-SUFFIX,githubusercontent.com,proxy

  # GeoIP
  - GEOIP,CN,domestic

  # 最终规则
  - MATCH,proxy
```

---

## Clash 兼容配置

完全兼容 Clash/Mihomo 的配置格式。

```yaml
# Clash 兼容配置 - 可直接用于 Clash 客户端

# 端口
port: 7890
socks-port: 7891
mixed-port: 7890

# 网络
allow-lan: true
bind-address: "*"
mode: rule
log-level: info
ipv6: false

# 外部控制
external-controller: 127.0.0.1:9090

# DNS
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - 223.5.5.5
    - 119.29.29.29
  fallback:
    - 8.8.8.8
    - 1.1.1.1

# 代理
proxies:
  - name: "节点1"
    type: ss
    server: server1.example.com
    port: 443
    cipher: aes-256-gcm
    password: "password"

  - name: "节点2"
    type: vmess
    server: server2.example.com
    port: 443
    uuid: uuid
    alterId: 0
    cipher: auto
    tls: true

  - name: "节点3"
    type: trojan
    server: server3.example.com
    port: 443
    password: password

# 代理组
proxy-groups:
  - name: "代理"
    type: select
    proxies:
      - 节点1
      - 节点2
      - 节点3
      - DIRECT

  - name: "自动选择"
    type: url-test
    proxies:
      - 节点1
      - 节点2
      - 节点3
    url: "http://www.gstatic.com/generate_204"
    interval: 300

# 规则
rules:
  - DOMAIN-SUFFIX,google.com,代理
  - DOMAIN-SUFFIX,github.com,代理
  - GEOIP,CN,DIRECT
  - MATCH,代理
```

---

## 多协议配置

展示所有支持的协议配置。

```yaml
# 多协议配置示例

mixed-port: 7890
allow-lan: true
mode: rule

dns:
  enable: true
  enhanced-mode: fake-ip
  nameserver:
    - https://dns.alidns.com/dns-query

proxies:
  # ========== HTTP ==========
  - name: "http-proxy"
    type: http
    server: http.example.com
    port: 8080
    username: "user"
    password: "pass"

  # ========== SOCKS5 ==========
  - name: "socks5-proxy"
    type: socks5
    server: socks5.example.com
    port: 1080
    username: "user"
    password: "pass"
    udp: true

  # ========== Shadowsocks ==========
  - name: "ss-aead"
    type: ss
    server: ss.example.com
    port: 443
    cipher: aes-256-gcm
    password: "password"
    udp: true

  - name: "ss-2022"
    type: ss
    server: ss2022.example.com
    port: 443
    cipher: 2022-blake3-aes-256-gcm
    password: "base64-encoded-key"

  # ========== VMess ==========
  - name: "vmess-tcp"
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: "uuid"
    alter-id: 0
    cipher: auto
    tls: true
    sni: vmess.example.com

  - name: "vmess-ws"
    type: vmess
    server: vmess-ws.example.com
    port: 443
    uuid: "uuid"
    alter-id: 0
    cipher: auto
    tls: true
    network: ws
    ws-path: /vmess
    ws-headers:
      Host: vmess-ws.example.com

  # ========== VLESS ==========
  - name: "vless-vision"
    type: vless
    server: vless.example.com
    port: 443
    uuid: "uuid"
    tls: true
    sni: vless.example.com
    flow: xtls-rprx-vision

  - name: "vless-grpc"
    type: vless
    server: vless-grpc.example.com
    port: 443
    uuid: "uuid"
    tls: true
    sni: vless-grpc.example.com
    network: grpc
    grpc-service-name: vless

  # ========== Trojan ==========
  - name: "trojan-tcp"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "password"
    sni: trojan.example.com
    udp: true

  - name: "trojan-ws"
    type: trojan
    server: trojan-ws.example.com
    port: 443
    password: "password"
    sni: trojan-ws.example.com
    network: ws
    ws-path: /trojan

  # ========== Hysteria2 ==========
  - name: "hysteria2"
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: "password"
    sni: hy2.example.com
    up: "100 Mbps"
    down: "100 Mbps"

  # ========== TUIC ==========
  - name: "tuic"
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: "uuid"
    password: "password"
    alpn:
      - h3
    congestion-controller: bbr
    sni: tuic.example.com

  # ========== WireGuard ==========
  - name: "wireguard"
    type: wireguard
    server: wg.example.com
    port: 51820
    private-key: "private-key"
    public-key: "public-key"
    mtu: 1400

proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - http-proxy
      - socks5-proxy
      - ss-aead
      - vmess-tcp
      - vless-vision
      - trojan-tcp
      - hysteria2
      - tuic
      - wireguard
      - DIRECT

rules:
  - MATCH,proxy
```

---

## 企业级配置

适合企业环境的完整配置。

```yaml
# 企业级配置

# ========== 基础配置 ==========
mixed-port: 7890
allow-lan: true
bind-address: "0.0.0.0"
mode: rule
log-level: warning

# ========== 外部控制 ==========
external-controller: 0.0.0.0:9090
secret: "your-api-secret"

# ========== DNS 配置 ==========
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16

  # 内部 DNS
  default-nameserver:
    - 10.0.0.1
    - 10.0.0.2

  # 外部 DNS
  nameserver:
    - https://dns.company.local/dns-query

  # 内部域名解析
  nameserver-policy:
    "*.company.local": "10.0.0.1"
    "*.internal.local": "10.0.0.1"

# ========== 规则提供者 ==========
rule-providers:
  company-direct:
    type: http
    behavior: domain
    url: "https://rules.company.local/direct.yaml"
    path: ./rules/company-direct.yaml
    interval: 3600

  company-proxy:
    type: http
    behavior: domain
    url: "https://rules.company.local/proxy.yaml"
    path: ./rules/company-proxy.yaml
    interval: 3600

  reject:
    type: http
    behavior: domain
    url: "https://cdn.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/reject.txt"
    path: ./ruleset/reject.yaml
    interval: 86400

# ========== 代理提供者 ==========
proxy-providers:
  company-nodes:
    type: http
    url: "https://nodes.company.local/proxies.yaml"
    path: ./providers/company.yaml
    interval: 3600
    health-check:
      enable: true
      url: "https://health.company.local/check"
      interval: 300

# ========== 代理组 ==========
proxy-groups:
  - name: "proxy"
    type: select
    use:
      - company-nodes
    proxies:
      - auto
      - DIRECT

  - name: "auto"
    type: url-test
    use:
      - company-nodes
    url: "https://health.company.local/check"
    interval: 300

# ========== 规则 ==========
rules:
  # 广告拦截
  - RULE-SET,reject,REJECT

  # 公司规则
  - RULE-SET,company-direct,DIRECT
  - RULE-SET,company-proxy,proxy

  # 私有网络
  - IP-CIDR,10.0.0.0/8,DIRECT
  - IP-CIDR,172.16.0.0/12,DIRECT
  - IP-CIDR,192.168.0.0/16,DIRECT

  # 最终规则
  - MATCH,proxy
```

---

## 透明代理配置

使用 TUN 模式的透明代理配置。

```yaml
# 透明代理配置 (TUN 模式)

mixed-port: 7890
allow-lan: true
mode: rule

# ========== TUN 配置 ==========
tun:
  enable: true
  stack: mixed              # system / gvisor / mixed
  dns-hijack:
    - any:53
  auto-route: true          # 自动设置路由表
  auto-detect-interface: true
  strict-route: true        # 严格路由
  mtu: 9000

# ========== DNS ==========
dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query

# ========== 代理 ==========
proxies:
  - name: "proxy-node"
    type: vless
    server: proxy.example.com
    port: 443
    uuid: "uuid"
    tls: true
    sni: proxy.example.com
    flow: xtls-rprx-vision

proxy-groups:
  - name: "proxy"
    type: select
    proxies:
      - proxy-node
      - DIRECT

rules:
  # 直连局域网
  - IP-CIDR,192.168.0.0/16,DIRECT
  - IP-CIDR,10.0.0.0/8,DIRECT

  # 代理
  - GEOIP,CN,DIRECT
  - MATCH,proxy
```

---

## 使用提示

1. **配置验证**: `hades -c config.yaml -t`
2. **调试模式**: `hades -c config.yaml -d`
3. **日志查看**: `hades-ctl logs`
4. **配置重载**: `hades-ctl reload` (部分配置支持热重载)
