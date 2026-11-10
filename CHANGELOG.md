# Changelog

Versions listed here are prepared release notes, not confirmation of publication.
这里记录待发布的版本说明，不代表版本已经发布。

## [0.1.1]

### 中文

修复首个预览版使用中的更新和配置初始化问题：

- 修复启动阶段版本状态偶发显示为空、无法检查更新的问题。
- 首次启动选择 JSON 配置文件位置，后续启动自动使用该位置。
- 明文密码与连接配置一起保存到所选 JSON；加密密码继续使用系统安全存储保护密钥。
- 修复跨平台 CI、发布产物汇总和测试环境兼容性问题。

### English

Fixes for update checks and configuration initialization in the first preview release:

- Retry the initial update status call so the current version is always shown.
- Choose the JSON configuration file location on first launch and remember it afterwards.
- Store plaintext passwords with the selected JSON configuration; encrypted passwords keep their key in the OS credential store.
- Fix cross-platform CI, release asset aggregation, and test-environment compatibility issues.

## [0.1.0]

### 中文

Tunnelix 首个桌面版本，支持简体中文和英文。

- 管理 SSH Local、Remote 和 SOCKS 转发，支持搜索、批量操作和托盘菜单。
- 支持 SSH Agent、私钥和密码认证；密码可不保存、明文保存或通过系统安全存储加密保存。
- 在配置表单中测试未保存的连接，查看和复制 SSH 命令。
- 自定义连接超时、保活间隔和最大连续无响应次数，默认 15 秒 / 15 秒 / 3 次。
- 支持断线重连、连接事件、主机指纹确认和不含凭据的配置导入导出。
- 提供版本检查与可关闭的自动下载，验证签名后由用户确认重启安装。

### English

The first Tunnelix desktop release, in English and Simplified Chinese.

- Manage SSH local, remote, and SOCKS forwarding with search, batch controls, and a system tray menu.
- Authenticate with SSH Agent, private keys, or passwords; keep passwords session-only, save plaintext, or encrypt with an OS-protected key.
- Test unsaved connections and preview/copy SSH commands.
- Customize connection timeout, keepalive interval, and unanswered keepalive limit; defaults are 15 seconds / 15 seconds / 3.
- Reconnect after network interruptions, inspect connection events, confirm host fingerprints, and import/export configuration without credentials.
- Check for versions and optionally download updates automatically, with signature verification and user-controlled restart.

### Requirements / 运行要求

- macOS 14 Sonoma+ (Intel or Apple Silicon); Windows 10 22H2+ / Windows 11 x64 with WebView2 and OpenSSH Client; Ubuntu 24.04 LTS-compatible Linux with GTK 4, WebKitGTK 6.0, and OpenSSH.
- Linux encrypted password storage needs a Secret Service provider.
- Multi-factor / keyboard-interactive authentication, terminal, SFTP, and OS login startup are not included.
- The declared minimum systems and physical desktop behaviour still require release qualification; see [platform validation status](docs/development.md#平台验证状态) before publishing.
