#!/bin/bash
set -uo pipefail
release_cleanup_status=0
python3 - <<'PY' || release_cleanup_status=$?
import os, shlex, subprocess
from pathlib import Path
saved=Path(os.environ['RUNNER_TEMP'])/'tunnelix-original-keychains.txt'
if saved.exists():
    subprocess.run(['security','list-keychains','-d','user','-s',*shlex.split(saved.read_text())],check=True)
    saved.unlink()
PY
if [ -n "${TUNNELIX_MAC_KEYCHAIN:-}" ] && [ -f "$TUNNELIX_MAC_KEYCHAIN" ]; then security delete-keychain "$TUNNELIX_MAC_KEYCHAIN" || release_cleanup_status=$?; fi
if [ -n "${TUNNELIX_SIGNING_DIR:-}" ]; then rm -rf -- "$TUNNELIX_SIGNING_DIR"; fi

exit "$release_cleanup_status"
