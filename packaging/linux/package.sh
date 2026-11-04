#!/bin/bash
# Linux amd64 and arm64 packages: a tarball (da, the dap launcher, plugins)
# and a .deb when dpkg-deb is installed.
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

if [[ "$version" == *-* ]]; then
  deb_version="${version%%-*}~${version#*-}"
else
  deb_version="$version"
fi

commit=$(git -C "$root" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X github.com/vicgupta/dap/internal/version.Release=${version} -X github.com/vicgupta/dap/internal/version.Commit=${commit} -X github.com/vicgupta/dap/internal/version.Date=${build_date}"

work="$root/dist/work/linux"
out="$root/dist/release"
rm -rf "$work"
mkdir -p "$work" "$out"

cat > "$work/INSTALL.txt" << EOF
DAP ${version} for Linux
========================

This archive contains:

  da        the CLI
  dap       launches the UI (same as: da run)
  plugins/  example plugins (need python3)

Data stays in ./.dap in this folder, or in \$DAP_HOME when that is set (a relative value is resolved from this folder).

install.sh copies da into the current directory, checks SHA256, and does not
change PATH. From that directory:

  ./da run

Run both commands from this folder:

  ./da run

A .deb is also published (dap_${deb_version}_amd64.deb and the arm64 build).
install.sh does not use it. The install that stays in this folder is ./da.

dap opens the URL in the default browser (xdg-open). Chrome is not required.
da run --no-browser only prints the URL.

Example plugins are in the plugins directory next to this archive:

  ./da plugin install ./plugins/mail-fake --approve
  ./da plugin install ./plugins/inbox-followups --approve

Gmail and Outlook need an OAuth client id. The demo mailbox does not.
EOF

build_one() {
  local arch="$1"
  local stage="$work/stage-$arch"
  rm -rf "$stage"
  mkdir -p "$stage/plugins"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$stage/da" ./cmd/da
  cp "$root/packaging/linux/dap" "$stage/dap"
  chmod 755 "$stage/da" "$stage/dap"
  cp -a "$root/plugins/." "$stage/plugins/"
  cp "$work/INSTALL.txt" "$stage/INSTALL.txt"
  cp "$root/LICENSE" "$stage/LICENSE"
  tar -C "$stage" -czf "$out/da-${version}-linux-${arch}.tar.gz" da dap plugins INSTALL.txt LICENSE
  build_deb "$arch" "$stage"
}

build_deb() {
  local arch="$1"
  local stage="$2"
  if ! command -v dpkg-deb >/dev/null 2>&1; then
    echo "dpkg-deb not found; skipping the ${arch} .deb" >&2
    return 0
  fi
  local pkg="$work/deb-$arch"
  rm -rf "$pkg"
  mkdir -p "$pkg/DEBIAN" "$pkg/usr/bin" "$pkg/usr/share/dap"
  cp "$stage/da" "$pkg/usr/bin/da"
  cp "$stage/dap" "$pkg/usr/bin/dap"
  chmod 755 "$pkg/usr/bin/da" "$pkg/usr/bin/dap"
  cp -a "$stage/plugins" "$pkg/usr/share/dap/plugins"
  local size
  size=$(du -sk "$pkg" | awk '{ print $1 }')
  cat > "$pkg/DEBIAN/control" << EOF
Package: dap
Version: ${deb_version}
Section: utils
Priority: optional
Architecture: ${arch}
Maintainer: Vic Gupta <vicgupta@users.noreply.github.com>
Installed-Size: ${size}
Homepage: https://github.com/vicgupta/dap
Description: local-first dashboard for personal agents
 The da command is the CLI. The dap command opens the local UI.
EOF
  dpkg-deb --root-owner-group --build "$pkg" "$out/dap_${deb_version}_${arch}.deb"
}

build_one amd64
build_one arm64
echo "Linux artifacts added to ${out}"
