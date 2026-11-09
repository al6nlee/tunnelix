# 安装与使用 / Installation and usage

当前为开发预览包，尚无正式签名发布。选择匹配系统与架构的安装包；`Tunnelix-update-*.zip` 仅供应用内更新。

Current packages are development previews. Choose your OS and architecture; `Tunnelix-update-*.zip` is only for the in-app updater.

## 安装 / Install

| 系统 / OS | 操作 / Steps |
| --- | --- |
| macOS | 打开 DMG，将 Tunnelix 拖到 Applications 后运行。/ Open the DMG and drag Tunnelix to Applications before launching. |
| Windows x64 | 安装 OpenSSH Client、WebView2，运行 `*-setup.exe`；默认安装到 `%LOCALAPPDATA%\Programs\Tunnelix`，无需管理员权限。也可解压 ZIP。/ Install OpenSSH Client and WebView2, then run the per-user installer or extract the ZIP. |
| Linux | 安装以下依赖，解压 tar.gz 后运行 `./tunnelix`。加密密码需要 GNOME Keyring 等 Secret Service。/ Install the dependencies below, extract the tarball, and run `./tunnelix`. Encrypted storage requires a Secret Service provider. |

Ubuntu 24.04：

```sh
sudo apt-get install libgtk-4-1 libwebkitgtk-6.0-4 openssh-client
```

Linux 桌面入口可选：将 `tunnelix.desktop` 的 `Exec`、`Icon` 改成绝对路径，复制到 `~/.local/share/applications/`。GNOME 显示托盘可能需要扩展。

For an optional Linux launcher, set absolute `Exec` and `Icon` paths in `tunnelix.desktop`, then copy it to `~/.local/share/applications/`. GNOME may need a tray extension.

## 连接 / Connect

1. 新建隧道，填写主机 / SSH 别名、用户名和认证方式。/ Create a tunnel and enter the host or SSH alias, user, and authentication method.
2. 添加 Local、Remote 或 SOCKS 转发，点击“测试连接”。测试使用当前未保存的表单，最长 2 分钟，会实际占用监听端口。/ Add forwarding rules and test. Testing uses the unsaved form, binds the configured ports, and times out after two minutes.
3. 核对主机指纹，保存并连接。成功只表示转发建立；目标服务仍需单独确认。/ Verify the fingerprint, save, and connect. Success confirms forwarding, not destination service health.

Local 的目标由服务器访问；Remote 的目标由本机访问，公网监听需服务端允许 `GatewayPorts`。SOCKS 可用 `socks5h://127.0.0.1:端口` 让服务器解析 DNS。

Local destinations are reached from the SSH server; Remote destinations from your computer. Public remote listeners require server-side `GatewayPorts`. Use `socks5h://127.0.0.1:port` for server-side DNS.

“高级设置”可调整以下整数参数；可一键恢复默认。/ Advanced settings accepts integers and offers a reset button.

| 参数 / Setting | 默认 / Default | 范围 / Range |
| --- | --- | --- |
| 连接超时 / Connect timeout | 15 秒 / seconds | 5–120 |
| 保活间隔 / Keepalive interval | 15 秒 / seconds | 5–300 |
| 连续无响应次数 / Unanswered keepalives | 3 | 1–10 |

快捷键 / Shortcuts：`Ctrl/Cmd+N` 新建 / New，`Ctrl/Cmd+F` 搜索 / Search，`Ctrl/Cmd+S` 保存 / Save。

## 配置与密码 / Configuration and passwords

配置保存在 JSON 中，写入时原子替换。/ Configuration is stored in an atomically replaced JSON file.

| 系统 / OS | 路径 / Path |
| --- | --- |
| macOS | `~/Library/Application Support/Tunnelix/config.json` |
| Windows | `%APPDATA%\Tunnelix\config.json` |
| Linux | `$XDG_CONFIG_HOME/Tunnelix/config.json`，默认 / default `~/.config/Tunnelix/config.json` |

密码可不保存、明文保存或加密保存。加密使用 AES-256-GCM，密钥留在系统安全存储；存储不可用时不会回退到明文。已有密码时留空可保留，选择“不保存”并保存可删除。修改连接身份后清除旧凭据。私钥口令不持久化。

Passwords can be session-only, plaintext, or AES-256-GCM encrypted with an OS-stored key. Encryption never falls back to plaintext. Leave the password blank to keep it; select “Do not save” and save to remove it. Changing connection identity clears saved credentials. Key passphrases are not persisted.

迁移时使用导出 / 导入，再输入密码；导出不含凭据。复制加密配置文件不会迁移系统密钥。导入合并隧道、关闭导入项的自动连接；ID 重复时整个导入失败。复制隧道也不复制密码和自动连接。

To migrate, export/import and re-enter passwords; exports exclude credentials. Copying encrypted configuration does not transfer the OS key. Imports merge tunnels with auto-connect disabled and reject duplicate IDs atomically. Duplicating a tunnel excludes its password and auto-connect setting.

## 更新与卸载 / Updates and uninstall

在“设置 → 版本与更新”手动检查，或保留默认的启动及每 6 小时自动检查、下载。可关闭自动更新。下载完成后需断开隧道、结束测试，再点击“重启并更新”。安装目录必须可写；未内置验签公钥的开发包只检查、不安装。

Check in Settings → Version & updates, or keep automatic checks/downloads at startup and every six hours. Automatic updates can be disabled. Disconnect tunnels and finish tests before choosing “Restart and update”. Installation requires a writable directory; development builds without a verification key cannot install updates.

卸载前从托盘退出（Linux 关闭窗口即退出）。Windows 使用系统“应用”设置或 `Uninstall.exe`；macOS 删除 `.app`；Linux 删除解压目录和自建桌面入口。更新和卸载均保留连接配置及系统凭据。

Quit before uninstalling. On Windows, use Apps settings or `Uninstall.exe`; on macOS, remove the `.app`; on Linux, remove the extracted directory and any launcher you added. Updates and uninstall preserve configuration and OS credentials.

下载后可将文件 SHA-256 与随包 `SHA256SUMS-<os>-<arch>` 对比：macOS 用 `shasum -a 256 <file>`，Linux 用 `sha256sum <file>`，Windows 用 `Get-FileHash <file> -Algorithm SHA256`。

Compare downloaded file hashes with `SHA256SUMS-<os>-<arch>` using the commands above.
