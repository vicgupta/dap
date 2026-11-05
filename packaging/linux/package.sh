#!/bin/bash
# Linux amd64 and arm64 CLI tarballs. Each archive contains only the da binary,
# same shape as the darwin tarballs (da-<version>-linux-<arch>.tar.gz).
# No .deb, launcher, or plugins bundle. Those are not release assets.
#
# Usage: packaging/linux/package.sh v0.1.0-alpha.2
# Writes into dist/release/ and does not delete other files there.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

version="${1:-}"
version="${version#v}"
if [ -z "$version" ]; then
  echo "usage: packaging/linux/package.sh v0.1.0-alpha.2" >&2
  exit 1
fi

commit=$(git -C "$root" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X github.com/vicgupta/dap/internal/version.Release=${version} -X github.com/vicgupta/dap/internal/version.Commit=${commit} -X github.com/vicgupta/dap/internal/version.Date=${build_date}"

work="$root/dist/work/linux"
out="$root/dist/release"
rm -rf "$work"
mkdir -p "$work" "$out"

build_one() {
  local arch="$1"
  local bin="$work/da-${arch}"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$bin" ./cmd/da
  local tmp
  tmp=$(mktemp -d)
  cp "$bin" "$tmp/da"
  chmod 755 "$tmp/da"
  tar -C "$tmp" -czf "$out/da-${version}-linux-${arch}.tar.gz" da
  rm -rf "$tmp"
}

build_one amd64
build_one arm64
echo "Linux artifacts added to ${out}"
