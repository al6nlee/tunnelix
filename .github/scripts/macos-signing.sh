#!/bin/bash
# Runs only on an ephemeral release runner; secrets must be provided by its environment.
set -euo pipefail
for name in MAC_CERTIFICATE_BASE64 MAC_CERTIFICATE_PASSWORD MAC_SIGN_IDENTITY MAC_NOTARY_KEY_BASE64 MAC_NOTARY_KEY_ID MAC_NOTARY_ISSUER; do
    if [ -z "${!name:-}" ]; then
        echo '::notice::macOS signing configuration is incomplete; building an ad-hoc signed preview.'
        echo 'TUNNELIX_ALLOW_UNSIGNED=true' >> "$GITHUB_ENV"
        exit 0
    fi
done
release_sign_dir=$(mktemp -d "$RUNNER_TEMP/tunnelix-signing.XXXXXX")
release_keychain="$release_sign_dir/release.keychain-db"
release_keychain_password=$(openssl rand -hex 24)
export TUNNELIX_SIGNING_DIR="$release_sign_dir"
printf 'TUNNELIX_SIGNING_DIR=%s\n' "$release_sign_dir" >> "$GITHUB_ENV"
printf 'TUNNELIX_MAC_KEYCHAIN=%s\n' "$release_keychain" >> "$GITHUB_ENV"
python3 - <<'PY'
import os, base64
from pathlib import Path
root=Path(os.environ['TUNNELIX_SIGNING_DIR'])
for env,name in [('MAC_CERTIFICATE_BASE64','certificate.p12'),('MAC_NOTARY_KEY_BASE64','notary.p8')]:
    path=root/name
    with path.open('xb') as f:
        os.chmod(path,0o600)
        f.write(base64.b64decode(os.environ[env],validate=True))
PY
# Record search paths before adding the temporary keychain, then restore in cleanup.
security list-keychains -d user > "$RUNNER_TEMP/tunnelix-original-keychains.txt"
security create-keychain -p "$release_keychain_password" "$release_keychain"
security set-keychain-settings -lut 21600 "$release_keychain"
security unlock-keychain -p "$release_keychain_password" "$release_keychain"
security import "$release_sign_dir/certificate.p12" -k "$release_keychain" -P "$MAC_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$release_keychain_password" "$release_keychain" >/dev/null
# Use Python argv construction so keychain paths with spaces remain intact.
python3 - "$release_keychain" <<'PY'
import os, shlex, subprocess, sys
from pathlib import Path
original=shlex.split((Path(os.environ['RUNNER_TEMP'])/'tunnelix-original-keychains.txt').read_text())
subprocess.run(['security','list-keychains','-d','user','-s',sys.argv[1],*original],check=True)
PY
xcrun notarytool store-credentials tunnelix-release --key "$release_sign_dir/notary.p8" --key-id "$MAC_NOTARY_KEY_ID" --issuer "$MAC_NOTARY_ISSUER" --keychain "$release_keychain"
printf 'TUNNELIX_MAC_SIGN_IDENTITY=%s\nTUNNELIX_MAC_NOTARY_PROFILE=tunnelix-release\n' "$MAC_SIGN_IDENTITY" >> "$GITHUB_ENV"
rm -f "$release_sign_dir/certificate.p12" "$release_sign_dir/notary.p8"
