#!/bin/bash
# Cross-compile the Windows CLI zips and check that each one is only da.exe.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
if ! command -v go >/dev/null 2>&1; then
  echo "go is required" >&2
  exit 1
fi
if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required" >&2
  exit 1
fi

cd "$root"
chmod +x packaging/windows/package.sh
packaging/windows/package.sh v0.0.0-ci
out="$root/dist/release"
flat="$root/dist/windows-cli"
rm -rf "$flat"
mkdir -p "$flat"

python3 - << PY
import zipfile
from pathlib import Path
out = Path("$out")
flat = Path("$flat")
names = [
    "da_windows_amd64.zip",
    "da_windows_arm64.zip",
    "da-0.0.0-ci-windows-amd64.zip",
    "da-0.0.0-ci-windows-arm64.zip",
]
for name in names:
    path = out / name
    if not path.is_file():
        raise SystemExit(f"missing {path}")
    z = zipfile.ZipFile(path)
    got = z.namelist()
    if got != ["da.exe"]:
        raise SystemExit(f"{name} contains {got}")
    data = z.read("da.exe")
    if data[:2] != b"MZ":
        raise SystemExit(f"{name} da.exe is not a PE")
    (flat / name).write_bytes(path.read_bytes())
print("ok  windows cli zips")
PY
