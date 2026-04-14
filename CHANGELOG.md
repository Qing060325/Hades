# 更新日志

所有重要的版本更新都会记录在此文件中。

## [v0.5.0] - 2026-04-14

### 🎉 重大更新：Windows 平台支持

#### 新增功能

| 功能 | 平台 | 说明 |
|------|------|------|
| **Windows Service** | Windows | 支持 install/uninstall/start/stop/restart/status 命令 |
| **PowerShell 安装脚本** | Windows | `install-hades.ps1` 自动下载、配置、安装服务 |
| **TUN 模式框架** | 全平台 | 统一的 TUN 设备抽象层 |

#### Windows Service 命令

```powershell
# 安装为 Windows Service
hades.exe install

# 卸载 Service
hades.exe uninstall

# 启动服务
hades.exe start

# 停止服务
hades.exe stop

# 重启服务
hades.exe restart

# 查看服务状态
hades.exe status
```

#### Windows 安装方式

**方式一：PowerShell 脚本（推荐）**
```powershell
irm https://raw.githubusercontent.com/Qing060325/Hades/main/install-hades.ps1 | iex
```

**方式二：手动安装**
```powershell
# 下载 Windows 版本
wget https://github.com/Qing060325/Hades/releases/latest/download/hades-windows-amd64.exe

# 移动到系统目录
move hades-windows-amd64.exe C:\hades\hades.exe

# 创建配置目录
New-Item -ItemType Directory -Force -Path C:\hades

# 安装为 Windows Service
C:\hades\hades.exe install

# 启动服务
C:\hades\hades.exe start
```

#### TUN 模式说明

| 平台 | TUN 模式 | 说明 |
|------|----------|------|
| Linux | ✅ 原生支持 | 使用 `/dev/net/tun` |
| macOS | ✅ 原生支持 | 使用 `utun` 设备 |
| Windows | ⚠️ 需要配置 | 推荐使用 HTTP 代理模式或 gVisor/Hyper-V |

**Windows TUN 替代方案：**
- **HTTP 代理模式**：`mixed-port` 模式，兼容所有应用
- **gVisor + Hades**：使用 gVisor 创建 TUN 设备
- **Hyper-V 虚拟交换机**：高级用户方案

#### 兼容性修复

- ✅ 修复 Windows 升级 API 文件路径问题（`/tmp` → `os.TempDir()`）
- ✅ 修复 Windows 文件替换逻辑（Rename → Copy + Delete）
- ✅ 修复跨平台信号处理（`syscall.Kill` → `os.FindProcess`）
- ✅ 修复订阅管理器未使用的 import

#### 构建改进

- ✅ 优化跨平台编译流程
- ✅ 支持 Windows Service 交叉编译
- ✅ GitHub Actions 多平台自动构建（linux-amd64/arm64, darwin-amd64/arm64, windows-amd64）

---

## [v0.4.0] - 2026-04-14

### 核心升级

- 🔄 **自动升级 API** - 支持通过 Web 面板一键升级
- 🔧 **订阅管理增强** - 自动更新订阅配置
- 📊 **性能监控** - 添加连接统计和流量监控
- 🐛 **Bug 修复** - 修复多个稳定性问题

---

## [v0.3.0] - 2026-04-13

### Web API 增强

- ✨ **RESTful API** - 完整的配置管理和运行时 API
- ✨ **健康检查** - `/health` 端点用于服务监控
- ✨ **配置热重载** - 无需重启即可更新配置

---

## [v0.2.1] - 2026-04-14

### Bug 修复

- 🐛 修复 hysteria2/tuic 适配器 ReadFrom 返回值问题
- 🐛 调整 Go 版本要求从 1.23 降至 1.21

### 脚本增强

- ✨ `hades_manager.sh` 添加下载超时、文件校验、重试机制
- 📦 提供全平台预编译二进制文件

---

## [v0.2.0] - 2026-04-13

### 新增协议支持

- ✨ Hysteria2 协议支持
- ✨ TUIC 协议支持
- ✨ WireGuard 协议支持

### 功能完善

- ✨ 实现 Clash YAML 完全兼容
- ✨ 一键安装脚本与交互式配置向导
- 🔒 TLS 1.3 安全连接支持
- 🔧 API 安全加固

---

## [v0.1.0] - 2026-04-10

### 初始版本

- 🎉 初始版本发布
- ✅ HTTP / SOCKS5 / Shadowsocks / VMess / VLESS / Trojan 协议支持
- ✅ 基础代理功能

---

<!--
版本格式说明：
- MAJOR: 不兼容的 API 变更
- MINOR: 向后兼容的新功能
- PATCH: 向后兼容的 Bug 修复

标签格式：[MAJOR.MINOR.PATCH] - YYYY-MM-DD
-->
