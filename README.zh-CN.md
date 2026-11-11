# Tunnelix

[English](README.md) · 简体中文

<p align="center">
  <img src="assets/brand/tunnelix-logo.png" width="280" alt="Tunnelix 标志">
</p>

轻量的桌面 SSH 隧道管理工具，基于 Go + Wails 3 + React，配置保存在本地 JSON 文件中。
支持 Windows、macOS、Linux，以及简体中文和英文。

系统要求：Windows 10 22H2 及以上（x64）、Windows 11（x64）、macOS 14 Sonoma 及以上（Intel 与 Apple Silicon 架构包），以及 Ubuntu 24.04 LTS 或兼容的 Linux 发行版。

**当前为 v0.2.2 开发预览版。** macOS arm64 和 Ubuntu 24.04 arm64 已完成本地构建与集成验证；Windows 目前通过交叉编译和安装器编译，尚未实机验证。

## 功能

- Local / Remote / SOCKS 转发、多端口映射、SSH 配置别名和跳板机。
- Agent、私钥、密码认证；密码可不保存、明文保存或由系统安全存储保护后加密保存。
- 紧凑连接列表与右侧详情、搜索与状态筛选、自动连接与断线重连。
- 浅色 / 深色主题，在设置中预览并保存。
- 托盘最多显示 10 条连接，运行中优先；更多连接从“查看全部”进入主窗口。
- 保存前测试连接、预览和复制 SSH 命令、网络参数自定义。
- 配置导入导出（不含凭据），可关闭的自动更新下载。

## 快速开始

安装包及运行依赖见[安装与使用](docs/install.md)。首次使用：先选择 JSON 配置文件位置，再新建隧道 → 填写服务器与认证信息 → 添加转发规则 → 测试连接 → 保存并连接。首次连接需核对主机指纹。

开发需要 Go 1.26.6+、Node.js 24、OpenSSH 和对应平台的桌面依赖，见[开发指南](docs/development.md)。在仓库根目录执行：

```sh
npm --prefix frontend ci
npm --prefix frontend run build
go run .
```

原生打包：

```sh
go run scripts/package.go
```

输出到 `dist/<版本>/<系统>-<架构>/`。正式签名、版本更新和 GitHub Release 操作见[发布指南](docs/releasing.md)。

## 使用边界

- 支持标准 SSH 密码认证；不支持 keyboard-interactive / MFA。跳板机建议使用 Agent / 密钥，私钥口令仅在会话中使用。
- “已连接”表示 SSH 转发已建立，不代表目标服务一定可用。
- macOS / Windows 关闭窗口后驻留托盘；Linux 关闭窗口即退出。“自动连接”指应用启动时连接。
- 配置使用 JSON，不使用数据库；位置、密码保存与迁移方法见[安装与使用](docs/install.md)。

## 许可证

[MIT](LICENSE)，Copyright (c) 2026 Alan。依赖许可见[第三方说明](THIRD_PARTY_NOTICES.md)。
