#!/bin/sh
# Exercise a disposable Secret Service collection without using the login keyring.
set -eu
if [ "${1:-}" != "--session" ]; then
  exec dbus-run-session -- sh "$0" --session
fi
keyring_test_root=$(mktemp -d)
export XDG_DATA_HOME="$keyring_test_root/data"
export XDG_CONFIG_HOME="$keyring_test_root/config"
export XDG_RUNTIME_DIR="$keyring_test_root/runtime"
mkdir -p "$XDG_DATA_HOME" "$XDG_CONFIG_HOME" "$XDG_RUNTIME_DIR"
chmod 700 "$XDG_RUNTIME_DIR"
trap 'gnome-keyring-daemon --quit >/dev/null 2>&1 || true; rm -rf "$keyring_test_root"' EXIT
printf '%s\n' 'tunnelix-disposable-test-keyring' | gnome-keyring-daemon --unlock --components=secrets > "$keyring_test_root/daemon.log"
TUNNELIX_KEYRING_TEST=1 go test -race -timeout 90s -run TestSystemKeyringRoundTrip -v ./internal/credentials
