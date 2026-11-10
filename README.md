# Tunnelix

English · [简体中文](README.zh-CN.md)

<p align="center">
  <img src="assets/brand/tunnelix-logo.png" width="280" alt="Tunnelix logo">
</p>

A lightweight desktop SSH tunnel manager built with Go + Wails 3 + React. Configuration stays in a local JSON file.
Supports Windows, macOS, Linux, English, and Simplified Chinese.

Supported systems: Windows 10 22H2 or later (x64), Windows 11 (x64), macOS 14 Sonoma or later (Intel and Apple Silicon builds), and Ubuntu 24.04 LTS or compatible Linux distributions.

**Currently a v0.1.2 development preview.** Local builds and integration checks have passed on macOS arm64 and Ubuntu 24.04 arm64. Windows has passed cross-compilation and installer compilation; runtime validation is pending.

## Features

- Local / Remote / SOCKS forwarding, multiple mappings, SSH aliases, and jump hosts.
- Agent, key, and password authentication; session-only, plaintext, or encrypted password storage backed by the OS credential store.
- Compact connection list and detail pane, search and status filters, automatic connection, and reconnection.
- Light / dark appearance, with preview and saved preferences in Settings.
- Tray shows up to 10 connections, active first; “View all” opens the main window for the rest.
- Connection testing before saving, SSH command previews, and configurable network settings.
- Credential-free configuration import/export and optional automatic update downloads.

## Quick start

See [installation and usage](docs/install.md) for packages and runtime requirements. Choose the JSON configuration file location, create a tunnel, enter server and authentication details, add forwarding rules, test, then save and connect. Verify the host fingerprint on first connection.

Development requires Go 1.26.6+, Node.js 24, OpenSSH, and platform desktop dependencies. See the [development guide](docs/development.md) (Chinese). Run from the repository root:

```sh
npm --prefix frontend ci
npm --prefix frontend run build
go run .
```

Build a native package:

```sh
go run scripts/package.go
```

Output goes to `dist/<version>/<os>-<arch>/`. See the [release guide](docs/releasing.md) (Chinese) for signing, update distribution, and GitHub Draft Releases.

## Scope

- Standard SSH password authentication is supported; keyboard-interactive / MFA is not. Use Agent / keys for jump hosts. Key passphrases remain session-only.
- “Connected” means forwarding is established, not that the destination service is reachable.
- Closing the window keeps macOS / Windows running in the tray; on Linux it exits. Auto-connect runs when the app starts.
- Configuration uses JSON, with no database. See [installation and usage](docs/install.md) for storage paths, passwords, and migration.

## License

[MIT](LICENSE), Copyright (c) 2026 Alan. See [third-party notices](THIRD_PARTY_NOTICES.md) for dependency licenses.
