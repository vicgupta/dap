#!/bin/sh
# Install the DAP CLI into the current directory.
#
#   curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
#   sh install.sh
#   ./da run
#
# Does not change PATH and does not use sudo.
# Set DAP_INSTALL_DIR, or pass --dir DIR, to install somewhere else.
# Downloads and the da binary stay in that folder. Data stays in ./.dap there.
set -eu

REPO="${DAP_REPO:-vicgupta/dap}"
TAG="${DAP_TAG:-v0.1.0-alpha.4}"
VER="${TAG#v}"

dest="${DAP_INSTALL_DIR:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --dir)
      shift
      dest="${1:-}"
      if [ -z "$dest" ]; then
        echo "usage: sh install.sh [--dir DIR]" >&2
        exit 1
      fi
      ;;
    --dir=*)
      dest="${1#--dir=}"
      ;;
    *)
      echo "unknown argument: $1" >&2
      echo "usage: sh install.sh [--dir DIR]" >&2
      exit 1
      ;;
  esac
  shift
done
if [ -z "$dest" ] || [ "$dest" = "." ]; then
  dest=$(pwd)
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to download DAP." >&2
  exit 1
fi

os=$(uname -s)
case "$os" in
  Darwin|Linux) ;;
  *)
    echo "This script is for macOS and Linux. On Windows use install.ps1." >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64|aarch64) goarch=arm64 ;;
  x86_64) goarch=amd64 ;;
  *)
    echo "Unsupported CPU: $(uname -m)." >&2
    exit 1
    ;;
esac

if [ "$os" = "Darwin" ]; then
  cli_tar="da-${VER}-darwin-${goarch}.tar.gz"
else
  cli_tar="da-${VER}-linux-${goarch}.tar.gz"
fi

workdir="${dest}/.dap/tmp/install.$$"
mkdir -p "$workdir"
trap 'rm -rf "$workdir"' EXIT

base="https://github.com/${REPO}/releases/download/${TAG}"
echo "Downloading ${TAG} ($(uname -s) ${goarch}) from ${base}"
if ! curl -fsSL -o "$workdir/checksums.txt" "${base}/checksums.txt"; then
  echo "Could not download checksums.txt from ${TAG}." >&2
  exit 1
fi
if ! curl -fsSL -o "$workdir/$cli_tar" "${base}/${cli_tar}"; then
  echo "Could not download ${cli_tar} from ${TAG}." >&2
  exit 1
fi
if [ ! -f "$workdir/$cli_tar" ]; then
  echo "Could not download ${cli_tar} from ${TAG}." >&2
  exit 1
fi

expected=$(awk -v f="$cli_tar" '$2 == f { print $1; exit }' "$workdir/checksums.txt")
if [ -z "$expected" ]; then
  echo "No SHA256 line for ${cli_tar} in checksums.txt" >&2
  exit 1
fi
if command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$workdir/$cli_tar" | awk '{ print $1 }')
else
  got=$(sha256sum "$workdir/$cli_tar" | awk '{ print $1 }')
fi
if [ "$got" != "$expected" ]; then
  echo "SHA256 mismatch for ${cli_tar}" >&2
  echo "  expected ${expected}" >&2
  echo "  got      ${got}" >&2
  exit 1
fi
echo "SHA256 ok  ${cli_tar}"

tar -xzf "$workdir/$cli_tar" -C "$workdir"
if [ ! -f "$workdir/da" ]; then
  echo "Archive ${cli_tar} does not contain da." >&2
  exit 1
fi

mkdir -p "$dest"
cp "$workdir/da" "$dest/da"
chmod 755 "$dest/da"
if [ "$os" = "Darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$dest/da" 2>/dev/null || true
fi

echo
echo "Installed da at ${dest}/da"
if [ "$dest" = "$(pwd)" ]; then
  echo "Run it from this folder:"
  echo "  ./da run"
else
  echo "Run it with:"
  echo "  \"${dest}/da\" run"
fi
echo "Data stays in ./.dap in the folder where you run da."
if [ -n "${DAP_HOME:-}" ]; then
  echo "DAP_HOME is set (${DAP_HOME}). A relative value is resolved from that folder."
fi
echo
echo "This installer does not change PATH and does not use sudo."
echo
echo "./da run opens the URL in the default browser. Chrome is not required. ./da run --no-browser only prints the URL."
