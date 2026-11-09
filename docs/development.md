# 开发指南

## 环境与运行

需要 Go 1.26.6+、Node.js 24、Git、OpenSSH，以及平台依赖。产品最低系统版本为 Windows 10 22H2（x64）、macOS 14 Sonoma、Ubuntu 24.04 LTS：

- macOS 14+：Xcode Command Line Tools。
- Windows 10 22H2+ / Windows 11（x64）：WebView2 Runtime、OpenSSH Client。
- Ubuntu 24.04：`sudo apt-get install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev openssh-client`。

在仓库根目录执行：

```sh
npm --prefix frontend ci
npm --prefix frontend run build
go run .
```

Go 嵌入 `frontend/dist`，修改前端后需重新构建。`npm --prefix frontend run dev` 仅预览界面，SSH 功能需要桌面应用。

## 结构与脚本

- `frontend/src/`：React / TypeScript 界面、i18next 中英文文案。
- `internal/tunnel/`、`askpass/`、`processguard/`：OpenSSH 调用、认证与进程清理。
- `internal/config/`、`credentials/`：JSON 配置、系统安全存储；不使用数据库。
- `internal/service/`：前后端接口；`updates/`、`release/`：更新与发布验证。
- `main.go`、`desktop.go`、`tray_menu.go`：桌面入口及平台集成。

macOS 常规 cgo 构建通过 `internal/mactray/` 保持原生托盘菜单绑定，避免跨屏点击时使用旧屏幕位置合成事件。该适配针对 Wails 3.1 的菜单型托盘；升级 Wails 时须运行 `go test ./internal/mactray` 并复核双屏点击，再决定是否移除适配。

脚本继续使用 Go，复用项目模块且无需额外运行环境；平台工具调用保留 Shell / PowerShell，安装器使用 NSIS。

| 命令 / 文件 | 用途 |
| --- | --- |
| `go run scripts/package.go` | 当前平台打包；正式发布加 `-release` |
| `go run scripts/release.go -dir <目录> -version X.Y.Z` | 离线校验产物并生成汇总校验和 |
| `go run scripts/release-notes.go -version X.Y.Z` | 提取 CHANGELOG 中的版本说明 |
| `go run scripts/update-keygen.go <仓库外私钥路径>` | 创建更新签名密钥 |
| `go run scripts/licenses.go` | 依赖变更后刷新根目录第三方许可证 |
| `go run scripts/icons.go` | 品牌原图变更后重新生成应用及托盘图标 |
| `scripts/sign-windows.ps1`、`.github/scripts/` | 发布时调用系统签名工具 |
| `scripts/installer.nsi` | Windows 安装器模板 |
| `scripts/Dockerfile.linux` | Ubuntu 隔离构建与测试环境 |

## 验证

先构建前端，再运行以下命令（Unix shell）：

```sh
go build -o bin/tunnelix .
TUNNELIX_TEST_HELPER="$PWD/bin/tunnelix" go test -race -timeout 10m ./internal/...
go vet ./...
npm --prefix frontend run format:check
./bin/tunnelix --smoke-test
```

Windows 使用 `bin/tunnelix.exe`，通过 PowerShell 的 `$env:TUNNELIX_TEST_HELPER` 设置绝对路径。Linux smoke 需要图形会话；无显示器时可用 `GTK_A11Y=none dbus-run-session -- xvfb-run -a ./bin/tunnelix --smoke-test`。

测试使用本地 SSH 服务和临时配置。手动 GUI 验证用 `./bin/tunnelix --config /绝对路径/临时/config.json`，不要使用真实连接配置。`--smoke-test` 自带隔离配置。

系统凭据测试按需运行：macOS / Windows 设置 `TUNNELIX_KEYRING_TEST=1` 后执行 `go test -race -run TestSystemKeyringRoundTrip ./internal/credentials`；Linux 安装 `gnome-keyring`、`dbus-x11` 后运行 `sh scripts/test-keyring-linux.sh`。设置 `TUNNELIX_SOAK_DURATION=2m` 可启用持续传输测试。

## 产物目录

| 路径 | 内容 | 清理 |
| --- | --- | --- |
| `dist/` | 版本化交付包 | 按需保留版本 |
| `bin/`、`build/` | 开发可执行文件、打包暂存和测试日志 | 可删除，下次构建再生成 |
| `frontend/dist/` | Go 编译所需的前端资源 | 删除后先执行前端构建 |
| `frontend/*.tsbuildinfo` | TypeScript 增量缓存 | 可删除 |
| `frontend/node_modules/` | 本地依赖 | 删除后执行 `npm --prefix frontend ci` |

以上均忽略提交；源码测试、品牌资源和根目录许可证属于项目文件，需要保留。

## 平台验证状态

截至 2026-11-09：macOS arm64、Ubuntu 24.04 arm64 容器通过构建、race / SSH 集成、系统凭据存取、打包及桌面 smoke；Windows amd64 通过交叉编译和 NSIS 安装器编译。

发布前仍需验证：Windows 实机安装 / 升级 / 卸载与连接；正式签名更新；Linux 实体桌面及 amd64；macOS 14 / Intel 实机运行；实际休眠唤醒及网络切换。托盘视觉效果和新增网络设置尚未完成完整 GUI 验收。
