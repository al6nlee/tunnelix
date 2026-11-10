# 发布指南

当前仅有本地开发预览包；尚未执行远程发布或正式签名。平台验证状态见[开发指南](development.md#平台验证状态)。

## 本地打包

在目标系统原生运行，先安装[开发依赖](development.md)：

```sh
npm --prefix frontend ci
npm --prefix frontend run build
go run scripts/package.go
go run scripts/release.go -dir dist/0.1.1/darwin-arm64 -version 0.1.1
```

按实际版本、系统和架构修改校验目录。默认版本位于 `internal/buildinfo/info.go`，可用环境变量 `TUNNELIX_VERSION=X.Y.Z` 覆盖。

| 系统 | 安装包 | 要求 |
| --- | --- | --- |
| macOS amd64 / arm64 | DMG、ZIP | Xcode 命令行工具 |
| Windows amd64 | setup.exe、ZIP | 安装器需要 NSIS 的 `makensis`；开发模式缺少时只生成 ZIP |
| Linux amd64 / arm64 | tar.gz | 对应架构的原生 Linux 环境 |

输出到 `dist/X.Y.Z/<os>-<arch>/`，包含完整安装包、更新 ZIP、发布元数据和 SHA-256 校验文件。构建会覆盖同版本、同平台输出目录。`bin/`、`build/` 是可清理的中间目录。

## 发布材料：更新密钥必填，平台证书可选

| 材料 | 用途 | 从哪里获取 |
| --- | --- | --- |
| 更新签名密钥（Ed25519） | 让 Tunnelix 验证下载的更新是否可信，三端共用 | 用下面的项目命令生成，无需购买 |
| macOS Developer ID 证书 + 公证凭据 | 签名应用并提交 Apple 公证 | Apple Developer Program 账户；见下方步骤 |
| Windows 代码签名证书 | 为 EXE / 安装器做 Authenticode 签名 | 向受 Windows 信任的证书机构申请 |

**临时发布只需配置更新公钥、私钥。macOS / Windows 证书可以留空。** 工作流按配置自动选择：

| 平台配置 | 生成的包 |
| --- | --- |
| macOS 证书、公证参数全部齐全 | Developer ID 签名并完成公证 |
| macOS 上述参数缺少任意一项 | ad-hoc 签名预览包，无 Apple 公证 |
| Windows PFX 未配置 | 未签名预览包 |
| Windows PFX 已配置 | 使用证书签名；PFX 有密码时须同时提供密码 |

任一平台缺少签名 / 公证时，草稿自动标记为 **Prerelease** 并注明平台；发布后需手动下载安装，应用自动更新会跳过。全部齐全时生成稳定版草稿。证书已配置但无效、密码错误或公证失败仍会报错，不会掩盖配置问题。

### 1. 生成更新密钥（只做一次）

在仓库外选择一个已存在的安全目录，执行：

```sh
go run scripts/update-keygen.go /absolute/secure/location/tunnelix-update.key
```

- 终端输出 `Public key: ...`：只复制冒号后的 Base64 值，作为公钥。
- `tunnelix-update.key` 文件内容：已经是 Base64 私钥，直接作为私钥值，**不要再编码一次**。
- 公钥可公开；私钥文件不得提交。后续版本沿用同一对密钥，换公钥会导致旧版本无法验证新更新。

### 2. 可选：准备 macOS 材料

1. 在 [Apple Developer 的 Certificates](https://developer.apple.com/account/resources/certificates/list) 中申请 **Developer ID Application** 证书，安装到生成 CSR 的 Mac 钥匙串中。在“钥匙串访问”导出证书及其私钥为 `.p12`，设置非空导出密码。不要选 Apple Development 或 Developer ID Installer。
2. 执行 `security find-identity -v -p codesigning`，找到 `Developer ID Application: 姓名或公司 (TEAMID)`，复制完整名称作为签名身份。
3. 在 [App Store Connect](https://appstoreconnect.apple.com/) 的 Users and Access → Integrations → App Store Connect API 中，创建具备公证权限的 **Team API Key**。下载 `.p8`，记下 **Key ID** 和 **Issuer ID**。当前脚本要求 Issuer ID，因此使用团队密钥。

### 3. 可选：准备 Windows 材料

需要包含证书及私钥的 `.pfx` 文件，以及它的导出密码。**当前脚本只支持 PFX 文件签名**；证书若由硬件令牌、HSM 或云签名服务保管私钥，需先适配签名脚本，不能用只含公钥的 `.cer` 代替。申请前先确认签名方式是否兼容。

## GitHub Draft Release：配置一次，以后推 Tag

### 1. 填写 Variables 和 Secrets

仓库 **Settings → Secrets and variables → Actions → Variables**，添加：

| Variable | 用途 / 填写内容 | 必填 |
| --- | --- | --- |
| `TUNNELIX_UPDATE_PUBLIC_KEY` | 上面生成的 Base64 公钥；只填值，不带 `Public key:` | 是 |
| `MAC_SIGN_IDENTITY` | 完整签名身份，例如 `Developer ID Application: Example Ltd (ABCDE12345)`；须与 `.p12` 中的证书一致 | macOS 签名时 |

仓库 **Settings → Environments → New environment**，创建名为 **`release`** 的 Environment，在其中添加 Secrets：

| Secret | 用途 / 填写内容 | 必填 |
| --- | --- | --- |
| `TUNNELIX_UPDATE_PRIVATE_KEY` | `tunnelix-update.key` 文件的原始文本内容 | 是 |
| `MAC_CERTIFICATE_BASE64` | 包含私钥的 `.p12` 文件整体做 Base64 编码 | macOS 签名时 |
| `MAC_CERTIFICATE_PASSWORD` | 导出 `.p12` 时设置的非空密码 | macOS 签名时 |
| `MAC_NOTARY_KEY_BASE64` | Apple `.p8` 文件整体做 Base64 编码 | macOS 签名时 |
| `MAC_NOTARY_KEY_ID` | Apple 显示的 Key ID，例如 `ABC123DEFG` | macOS 签名时 |
| `MAC_NOTARY_ISSUER` | Apple 显示的 Issuer ID，UUID 格式；不是 Team ID | macOS 签名时 |
| `WINDOWS_CERTIFICATE_BASE64` | 包含私钥的 `.pfx` 文件整体做 Base64 编码 | Windows 签名时 |
| `WINDOWS_CERTIFICATE_PASSWORD` | `.pfx` 的导出密码；仅当 PFX 本身无密码时可留空 | 有密码时必填 |

`.p12`、`.p8`、`.pfx` 的 Base64 值可用以下命令生成（替换文件路径）：

```sh
python3 -c 'import base64,pathlib,sys; print(base64.b64encode(pathlib.Path(sys.argv[1]).read_bytes()).decode())' /path/to/certificate.p12
```

复制整行输出到对应 Secret，不加引号、前缀或换行。示例中的身份、ID、路径都需替换成自己的值，不能直接照填。

可选设置：给 `release` Environment 配置审批人；如限制部署分支 / Tag，需允许 `v*` Tag，以及手动运行所选的 ref。工作流自动提供 `GITHUB_TOKEN`，无需额外配置 PAT。

### 2. 每次发布

1. 更新 `CHANGELOG.md`，例如新增 `## [0.1.1]`，完成验证后提交并推送代码。
2. 在该提交上创建并推送版本 Tag（下面版本仅为示例）：

   ```sh
   git tag v0.1.1
   git push origin v0.1.1
   ```

3. 在 **Actions → Prepare release** 查看结果：三端测试 → 按证书配置打包 → 汇总校验 → 创建 **Draft Release**。
4. 在 **Releases** 中打开草稿，下载验证安装包，确认后点击 **Publish release**。发布前，应用内更新不会发现这个版本。

| 输入 / 设置 | 允许的值 |
| --- | --- |
| Git Tag | `vX.Y.Z`，例如 `v0.1.1`；不支持 `v1`、`v01.0.0` 或 `v0.1.1-beta.1` |
| 手动 Run workflow 的 `version` | `X.Y.Z`，例如 `0.1.1`；不带 `v`，对应 Tag 必须已存在 |
| 草稿 / 预发布 | 始终生成草稿；平台签名 / 公证不齐时自动标记 Prerelease，无需填写开关 |
| 构建平台 | 当前固定 Windows、macOS、Linux；各 runner 的原生架构，暂无平台选择参数 |

失败时可在 Actions 重跑，或手动输入同一版本号。构建始终固定到该 Tag 的提交，已有同名 Release（含草稿）时拒绝覆盖。普通分支推送 / PR 只运行 CI 并上传 Artifacts，不创建 Release。

## 可选：在本机做正式签名

使用 GitHub 发布时可跳过本节，工作流会自动设置这些环境变量。本机发布需先完成前端构建，保持 Git 工作区干净，再设置：

| 环境变量 | 填写内容 / 来源 | 本机必填范围 |
| --- | --- | --- |
| `TUNNELIX_VERSION` | `X.Y.Z`；留空使用 `internal/buildinfo/info.go` 的版本 | 可选，建议显式指定 |
| `TUNNELIX_UPDATE_PUBLIC_KEY` | 与 GitHub 同名 Variable 相同的值 | 三端 |
| `TUNNELIX_UPDATE_PRIVATE_KEY` | 与 GitHub 同名 Secret 相同的值 | 三端 |
| `TUNNELIX_MAC_SIGN_IDENTITY` | 与 `MAC_SIGN_IDENTITY` 相同；证书及私钥须已安装到钥匙串 | macOS |
| `TUNNELIX_MAC_NOTARY_PROFILE` | 本机 `notarytool` 凭据名称，例如 `tunnelix-release`；创建方式见下方 | macOS |
| `TUNNELIX_MAC_KEYCHAIN` | 公证凭据所在的自定义钥匙串路径；留空使用默认钥匙串 | 可选，仅 macOS |
| `TUNNELIX_WINDOWS_CERTIFICATE` | `.pfx` 的本机绝对路径，**不是 Base64 内容** | Windows |
| `TUNNELIX_WINDOWS_CERTIFICATE_PASSWORD` | PFX 密码，对应 GitHub 的 `WINDOWS_CERTIFICATE_PASSWORD` | Windows，PFX 有密码时 |

macOS 首次保存公证凭据（名称可自定，须与 profile 环境变量一致）：

```sh
xcrun notarytool store-credentials tunnelix-release \
  --key /path/to/AuthKey_ABC123DEFG.p8 \
  --key-id ABC123DEFG \
  --issuer YOUR_ISSUER_UUID
```

使用自定义钥匙串时，此命令增加 `--keychain /path/to/keychain-db`，并将相同路径填入 `TUNNELIX_MAC_KEYCHAIN`。Windows 还需安装 Windows SDK 的 `signtool.exe` 和 NSIS，均须在 PATH 中。Linux 不需要额外平台证书。

设置环境变量时，macOS / Linux 使用 `export 名称='值'`，Windows PowerShell 使用 `$env:名称 = '值'`。然后执行：

```sh
go run scripts/package.go -release
```

`-release` 默认要求平台签名及公证。本地要构建临时预览包，可用 `go run scripts/package.go -release -allow-unsigned`；仍要求干净的 Git 提交、更新密钥和完整安装包。macOS 未配置身份时使用 ad-hoc 签名。GitHub 工作流会自动选择此参数，无需手动设置。

离线汇总校验（产物放在同一目录，使用可信公钥环境变量）：

```sh
go run scripts/release.go -dir release-assets -version X.Y.Z \
  -platforms darwin,windows,linux -commit EXPECTED_COMMIT -production
```

`-dir` 是产物目录；`-version` 是目标版本；`-platforms` 可填 `darwin`、`windows`、`linux` 的逗号组合；`-commit` 是预期构建提交的完整 SHA（`git rev-parse HEAD`）；`-production` 启用正式产物的严格校验。验证临时预览包时再加 `-allow-unsigned`，仅放宽平台签名 / 公证要求，更新签名、来源提交、校验和及安装包完整性仍会检查。

## 应用内更新约定

- 使用 `al6nlee/tunnelix` 最新稳定 Release，标签为 `vX.Y.Z`，跳过 draft、prerelease、同版本和降级。
- 每个平台上传 `Tunnelix-update-<os>-<arch>.zip` 及同名 `.update.json` 签名清单；签名后不要修改归档。
- 更新 ZIP 顶层仅包含 `.app`（macOS）或可执行文件（Windows / Linux）。先完成系统签名 / 公证，再签更新清单。
- 用户确认重启后才安装；有活动连接 / 测试时阻止重启。配置保留，安装目录须可写。
- Windows / Linux 仅替换可执行文件；安装记录和旁置文件通过完整安装包刷新。当前依赖许可证已嵌入程序。
- 无验签公钥、`--config` 隔离实例、macOS 独立二进制仅检查、不安装。暂存只在当前进程有效；helper 在替换或启动失败时尝试恢复，不做新版本业务健康检查。
