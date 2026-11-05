#!/bin/bash
# The Linux release archives are static CLI tarballs: one file, named da.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

rm -rf "$root/dist/release" "$root/dist/work/linux"
packaging/linux/package.sh v0.0.0-test

out="$root/dist/release"
amd64="$out/da-0.0.0-test-linux-amd64.tar.gz"
arm64="$out/da-0.0.0-test-linux-arm64.tar.gz"

if [ ! -f "$amd64" ] || [ ! -f "$arm64" ]; then
  echo "missing linux tarballs in $out" >&2
  ls -la "$out" >&2
  exit 1
fi

for name in "$out"/*; do
  base=$(basename "$name")
  case "$base" in
    da-0.0.0-test-linux-amd64.tar.gz|da-0.0.0-test-linux-arm64.tar.gz) ;;
    *)
      echo "unexpected release file: $base" >&2
      exit 1
      ;;
  esac
done

for archive in "$amd64" "$arm64"; do
  entries=$(tar -tzf "$archive" | sort)
  if [ "$entries" != "da" ]; then
    echo "$archive contains more than da:" >&2
    printf '%s\n' "$entries" >&2
    exit 1
  fi
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tar -xzf "$amd64" -C "$tmp"
if ! "$tmp/da" version | grep -q '"ok": true'; then
  echo "linux/amd64 da version failed" >&2
  exit 1
fi
if ldd "$tmp/da" 2>&1 | grep -q '=>'; then
  echo "linux/amd64 da is dynamically linked" >&2
  ldd "$tmp/da" >&2 || true
  exit 1
fi

armtmp=$(mktemp -d)
tar -xzf "$arm64" -C "$armtmp"
if ! file "$armtmp/da" | grep -Eqi 'ARM|aarch64'; then
  echo "linux/arm64 archive is not an arm64 binary:" >&2
  file "$armtmp/da" >&2
  exit 1
fi
rm -rf "$armtmp"

echo "ok  linux package"
