#!/bin/bash
# Windows amd64 zip: da.exe, DAP.cmd (runs da.exe run), and plugins.
# Unsigned. No SmartScreen reputation and no Authenticode signature.
#
# Usage: packaging/windows/package.sh v0.1.0-alpha.2
# Writes into dist/release/ and does not delete other files there.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

version="${1:-}"
version="${version#v}"
if [ -z "$version" ]; then
  echo "usage: packaging/windows/package.sh v0.1.0-alpha.2" >&2
  exit 1
fi

commit=$(git -C "$root" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X github.com/vicgupta/dap/internal/version.Release=${version} -X github.com/vicgupta/dap/internal/version.Commit=${commit} -X github.com/vicgupta/dap/internal/version.Date=${build_date}"

work="$root/dist/work/windows"
out="$root/dist/release"
stage="$work/stage"
rm -rf "$work"
mkdir -p "$stage/plugins" "$out"

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$stage/da.exe" ./cmd/da
# CRLF so cmd.exe accepts the launcher.
printf '@echo off\r\n"%%~dp0da.exe" run\r\n' > "$stage/DAP.cmd"
cp -a "$root/plugins/." "$stage/plugins/"
cp "$root/LICENSE" "$stage/LICENSE"

cat > "$stage/INSTALL.txt" << EOF
DAP ${version} for Windows
==========================

This zip contains:

  da.exe     the CLI
  DAP.cmd    double-click to run the UI (same as: da.exe run)
  plugins\\  example plugins (need Python: python or py)

Data stays in .\\.dap in this folder, or in %DAP_HOME% when that is set (a relative value is resolved from this folder).

install.ps1 copies da.exe into the current directory, unblocks that copy,
and does not change PATH. From that directory:

  .\\da.exe run

SmartScreen
-----------
This exe is unsigned. Windows may say "Windows protected your PC" the
first time it runs, especially after a browser download.

Choose More info, then Run anyway.

In PowerShell, from the folder that contains da.exe:

  Unblock-File .\\da.exe
  Unblock-File .\\DAP.cmd

da.exe run opens the URL in the default browser. Chrome is not required.
da.exe run --no-browser only prints the URL.

Example plugins, from this folder:

  .\\da.exe plugin install .\\plugins\\mail-fake --approve
  .\\da.exe plugin install .\\plugins\\inbox-followups --approve

Gmail and Outlook need an OAuth client id. The demo mailbox does not.
EOF

(
  cd "$stage"
  zip -r -X "$out/DAP-${version}-windows-amd64.zip" da.exe DAP.cmd plugins INSTALL.txt LICENSE
)

# CLI zips are da.exe only. install.ps1 installs these into the current directory.
cli_zip() {
  local arch="$1"
  local bin="$2"
  local tmp
  tmp=$(mktemp -d)
  cp "$bin" "$tmp/da.exe"
  (cd "$tmp" && zip -X "$out/da-${version}-windows-${arch}.zip" da.exe)
  rm -rf "$tmp"
}
cli_zip amd64 "$stage/da.exe"
arm_stage="$work/arm64"
mkdir -p "$arm_stage"
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$ldflags" -o "$arm_stage/da.exe" ./cmd/da
cli_zip arm64 "$arm_stage/da.exe"
echo "Windows artifacts added to ${out}"
