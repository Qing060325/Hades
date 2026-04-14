<#
.SYNOPSIS
    Hades 代理内核 Windows 一键安装脚本

.DESCRIPTION
    自动下载、安装并配置 Hades 作为 Windows 服务运行

.PARAMETER Version
    指定安装版本（默认：latest）

.PARAMETER ConfigPath
    指定配置文件路径

.EXAMPLE
    # 安装最新版本
    irm https://raw.githubusercontent.com/Qing060325/Hades/main/install-hades.ps1 | iex

    # 安装指定版本
    irm https://raw.githubusercontent.com/Qing060325/Hades/main/install-hades.ps1 | iex -Version v0.4.0

.NOTES
    需要管理员权限运行
#>

[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$ConfigPath = ""
)

# 颜色定义
$Colors = @{
    Success = "Green"
    Error   = "Red"
    Warning = "Yellow"
    Info    = "Cyan"
}

function Write-Success { param($Message) Write-Host "[✓] $Message" -ForegroundColor $Colors.Success }
function Write-ErrorMsg { param($Message) Write-Host "[✗] $Message" -ForegroundColor $Colors.Error }
function Write-WarningMsg { param($Message) Write-Host "[!] $Message" -ForegroundColor $Colors.Warning }
function Write-Info { param($Message) Write-Host "[i] $Message" -ForegroundColor $Colors.Info }

# 检查管理员权限
function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# 获取最新版本信息
function Get-LatestVersion {
    $apiUrl = "https://api.github.com/repos/Qing060325/Hades/releases/latest"
    try {
        $response = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing
        return @{
            Tag = $response.tag_name
            Assets = $response.assets
        }
    } catch {
        Write-ErrorMsg "获取最新版本失败: $_"
        return $null
    }
}

# 获取下载 URL
function Get-DownloadUrl {
    param([hashtable]$ReleaseInfo, [string]$Version)

    $os = "windows"
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64" -or $env:PROCESSOR_ARCHITEW6432 -eq "AMD64") { "amd64" } else { "arm64" }
    $filename = "hades-$os-$arch.exe"

    if ($Version -ne "latest") {
        $tag = $Version
    } else {
        $tag = $ReleaseInfo.Tag
    }

    return "https://github.com/Qing060325/Hades/releases/download/$tag/$filename"
}

# 下载文件
function Invoke-Download {
    param([string]$Url, [string]$OutFile)

    Write-Info "正在下载: $Url"
    try {
        Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
        Write-Success "下载完成"
        return $true
    } catch {
        Write-ErrorMsg "下载失败: $_"
        return $false
    }
}

# 创建配置目录
function Initialize-ConfigDirectory {
    $configDir = "$env:USERPROFILE\.config\hades"
    $providersDir = "$configDir\providers"

    if (-not (Test-Path $configDir)) {
        New-Item -ItemType Directory -Path $configDir -Force | Out-Null
        Write-Success "创建配置目录: $configDir"
    }

    if (-not (Test-Path $providersDir)) {
        New-Item -ItemType Directory -Path $providersDir -Force | Out-Null
    }

    # 创建默认配置
    $configFile = "$configDir\config.yaml"
    if (-not (Test-Path $configFile)) {
        $defaultConfig = @"
# Hades 配置文件
# 混合端口 (HTTP + SOCKS5)
mixed-port: 7890

# 允许局域网连接
allow-lan: true

# 绑定地址
bind-address: "*"

# 运行模式: rule / global / direct
mode: rule

# 日志级别
log-level: info

# IPv6 支持
ipv6: true

# 外部控制器
external-controller: 0.0.0.0:9090

# DNS 配置
dns:
  enable: true
  ipv6: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  fallback:
    - https://cloudflare-dns.com/dns-query
    - https://dns.google/dns-query
  fallback-filter:
    geoip: true
    geoip-code: CN

# 代理配置（请添加你的节点）
proxies: []

# 代理组
proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - DIRECT
  - name: "Auto"
    type: url-test
    proxies: []
    url: "https://www.gstatic.com/generate_204"
    interval: 300

# 规则
rules:
  - DOMAIN-SUFFIX,cn,DIRECT
  - GEOIP,CN,DIRECT
  - MATCH,Proxy
"@
        $defaultConfig | Out-File -FilePath $configFile -Encoding UTF8
        Write-Success "创建配置文件: $configFile"
    }

    return $configFile
}

# 安装服务
function Install-HadesService {
    param([string]$ExePath, [string]$ConfigPath)

    $serviceName = "Hades"
    $displayName = "Hades Proxy Kernel"
    $description = "Hades 高性能代理内核服务"

    # 检查服务是否已存在
    $existingService = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($existingService) {
        Write-WarningMsg "服务已存在，正在停止并卸载..."
        Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
        sc.exe delete $serviceName | Out-Null
        Start-Sleep -Seconds 2
    }

    # 创建服务（使用 sc.exe 因为 New-Service 需要完整路径）
    $binPath = "`"$ExePath`" -c `"$ConfigPath`""
    $createResult = sc.exe create $serviceName binPath= $binPath start= auto DisplayName= $displayName
    if ($LASTEXITCODE -ne 0) {
        Write-ErrorMsg "创建服务失败: $createResult"
        return $false
    }

    # 设置描述
    sc.exe description $serviceName $description | Out-Null

    # 设置失败后自动重启
    sc.exe failure $serviceName reset= 86400 actions= restart/5000 | Out-Null

    Write-Success "服务安装成功"
    return $true
}

# 启动服务
function Start-HadesService {
    try {
        Start-Service -Name "Hades" -ErrorAction Stop
        Write-Success "服务已启动"
        return $true
    } catch {
        Write-ErrorMsg "启动服务失败: $_"
        return $false
    }
}

# 显示安装信息
function Show-InstallInfo {
    param([string]$Port, [string]$ConfigPath)

    Write-Host ""
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
    Write-Host "  Hades 代理内核安装完成！" -ForegroundColor Green
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  前端面板: http://localhost:$Port" -ForegroundColor White
    Write-Host "  API 地址: http://localhost:9090" -ForegroundColor White
    Write-Host ""
    Write-Host "  配置目录: $ConfigPath" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  常用命令:" -ForegroundColor Yellow
    Write-Host "    启动服务: Start-Service Hades" -ForegroundColor Gray
    Write-Host "    停止服务: Stop-Service Hades" -ForegroundColor Gray
    Write-Host "    查看状态: Get-Service Hades" -ForegroundColor Gray
    Write-Host "    查看日志: Get-WinEvent -FilterHashtable @{LogName='Application';ProviderName='Hades'}" -ForegroundColor Gray
    Write-Host "    卸载服务: sc.exe delete Hades" -ForegroundColor Gray
    Write-Host ""
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
}

# 主安装流程
function Install-Hades {
    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║       Hades Windows 安装脚本 v1.0           ║" -ForegroundColor Cyan
    Write-Host "╚══════════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host ""

    # 检查管理员权限
    if (-not (Test-Administrator)) {
        Write-ErrorMsg "需要管理员权限运行此脚本"
        Write-Info "请右键点击 PowerShell，选择 '以管理员身份运行'"
        exit 1
    }

    Write-Info "检测到 Windows 平台"
    Write-Info "版本: $Version"

    # 获取版本信息
    Write-Info "正在获取版本信息..."
    $releaseInfo = Get-LatestVersion
    if (-not $releaseInfo) {
        Write-ErrorMsg "无法获取版本信息"
        exit 1
    }

    $targetVersion = if ($Version -eq "latest") { $releaseInfo.Tag } else { $Version }
    Write-Info "目标版本: $targetVersion"

    # 下载 URL
    $downloadUrl = Get-DownloadUrl -ReleaseInfo $releaseInfo -Version $Version
    Write-Info "下载链接: $downloadUrl"

    # 临时目录
    $tempDir = Join-Path $env:TEMP "hades-install"
    if (Test-Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    # 下载二进制文件
    $exePath = Join-Path $tempDir "hades.exe"
    if (-not (Invoke-Download -Url $downloadUrl -OutFile $exePath)) {
        exit 1
    }

    # 初始化配置
    Write-Info "正在初始化配置..."
    $configFile = Initialize-ConfigDirectory

    # 如果指定了配置路径，使用指定的
    if ($ConfigPath) {
        $configFile = $ConfigPath
        Write-Info "使用指定配置: $configFile"
    }

    # 安装服务
    Write-Info "正在安装服务..."
    if (-not (Install-HadesService -ExePath $exePath -ConfigPath $configFile)) {
        exit 1
    }

    # 复制到永久位置
    $installDir = Join-Path $env:ProgramFiles "Hades"
    if (-not (Test-Path $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
    $permanentPath = Join-Path $installDir "hades.exe"
    Copy-Item -Path $exePath -Destination $permanentPath -Force
    Write-Success "安装文件复制到: $permanentPath"

    # 更新服务指向永久位置
    sc.exe config Hades binPath= "`"$permanentPath`" -c `"$configFile`"" | Out-Null

    # 启动服务
    Start-HadesService

    # 清理临时文件
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue

    # 显示信息
    Show-InstallInfo -Port 9090 -ConfigPath $configFile
}

# 执行安装
Install-Hades
