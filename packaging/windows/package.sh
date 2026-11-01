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

Data stays in %USERPROFILE%\\.dap, or in %DAP_HOME% when that is set.

install.ps1 copies these into %LOCALAPPDATA%\\DAP, unblocks them, puts
that folder on your user PATH, and adds a Start menu shortcut named DAP
that runs "da.exe run".

SmartScreen
-----------
This exe is unsigned. Windows may say "Windows protected your PC" the
first time it runs, especially after a browser download.

Choose More info, then Run anyway.

In PowerShell, from the folder that contains da.exe:

  Unblock-File .\\da.exe
  Unblock-File .\\DAP.cmd

The chromeless window needs Microsoft Edge or Google Chrome. Otherwise
DAP opens the URL in the default browser.

Example plugins, after da.exe is on PATH:

  da plugin install %LOCALAPPDATA%\\DAP\\plugins\\mail-fake --approve
  da plugin install %LOCALAPPDATA%\\DAP\\plugins\\inbox-followups --approve

Gmail and Outlook need an OAuth client id. The demo mailbox does not.
EOF

(
  cd "$stage"
  zip -r -X "$out/DAP-${version}-windows-amd64.zip" da.exe DAP.cmd plugins INSTALL.txt LICENSE
)
echo "Windows artifact added to ${out}"
