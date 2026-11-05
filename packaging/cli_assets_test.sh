#!/bin/bash
# The release directory must keep CLI assets and drop app bundles, debs, and launchers.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
# shellcheck source=packaging/cli_assets.sh
source "$root/packaging/cli_assets.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
out="$tmp/release"
mkdir -p "$out"

touch \
  "$out/da-0.1.0-test-darwin-arm64.tar.gz" \
  "$out/da-0.1.0-test-darwin-amd64.tar.gz" \
  "$out/da-0.1.0-test-darwin-universal.tar.gz" \
  "$out/da-0.1.0-test-linux-amd64.tar.gz" \
  "$out/da-0.1.0-test-linux-arm64.tar.gz" \
  "$out/da-0.1.0-test-windows-amd64.zip" \
  "$out/da-0.1.0-test-windows-arm64.zip" \
  "$out/install.sh" \
  "$out/install.ps1" \
  "$out/checksums.txt" \
  "$out/DAP-0.1.0-test-macos-arm64.zip" \
  "$out/DAP-0.1.0-test-macos-universal.zip" \
  "$out/DAP-0.1.0-test-windows-amd64.zip" \
  "$out/dap_0.1.0~test_amd64.deb" \
  "$out/dap_0.1.0~test_arm64.deb" \
  "$out/INSTALL.txt"

cli_release_prune "$out"

for gone in \
  DAP-0.1.0-test-macos-arm64.zip \
  DAP-0.1.0-test-macos-universal.zip \
  DAP-0.1.0-test-windows-amd64.zip \
  dap_0.1.0~test_amd64.deb \
  dap_0.1.0~test_arm64.deb \
  INSTALL.txt
do
  if [ -e "$out/$gone" ]; then
    fail "prune left $gone"
  fi
done

for kept in \
  da-0.1.0-test-darwin-arm64.tar.gz \
  da-0.1.0-test-darwin-amd64.tar.gz \
  da-0.1.0-test-darwin-universal.tar.gz \
  da-0.1.0-test-linux-amd64.tar.gz \
  da-0.1.0-test-linux-arm64.tar.gz \
  da-0.1.0-test-windows-amd64.zip \
  da-0.1.0-test-windows-arm64.zip \
  install.sh \
  install.ps1 \
  checksums.txt
do
  if [ ! -f "$out/$kept" ]; then
    fail "prune removed $kept"
  fi
done

cli_release_assert_clean "$out"
cli_release_require "$out" checksums

listed=$(cli_release_list "$out" | xargs -n1 basename | sort)
expected=$(printf '%s\n' \
  checksums.txt \
  da-0.1.0-test-darwin-amd64.tar.gz \
  da-0.1.0-test-darwin-arm64.tar.gz \
  da-0.1.0-test-darwin-universal.tar.gz \
  da-0.1.0-test-linux-amd64.tar.gz \
  da-0.1.0-test-linux-arm64.tar.gz \
  da-0.1.0-test-windows-amd64.zip \
  da-0.1.0-test-windows-arm64.zip \
  install.ps1 \
  install.sh | sort)
if [ "$listed" != "$expected" ]; then
  fail "upload list mismatch
got:
$listed
want:
$expected"
fi

rm -f "$out/da-0.1.0-test-linux-arm64.tar.gz"
if cli_release_require "$out" checksums; then
  fail "require succeeded without the linux arm64 tarball"
fi

echo "ok  cli release assets"
