#!/bin/bash
# Windows amd64 and arm64 CLI zips. Each archive contains only da.exe.
# Unsigned. No SmartScreen reputation and no Authenticode signature.
# No DAP.cmd launcher and no plugins bundle. Those are not release assets.
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
rm -rf "$work"
mkdir -p "$work" "$out"

cli_zip() {
  local arch="$1"
  local bin="$work/da-${arch}.exe"
  CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$bin" ./cmd/da
  local tmp
  tmp=$(mktemp -d)
  cp "$bin" "$tmp/da.exe"
  (cd "$tmp" && zip -X "$out/da-${version}-windows-${arch}.zip" da.exe)
  rm -rf "$tmp"
}

cli_zip amd64
cli_zip arm64
echo "Windows artifacts added to ${out}"
