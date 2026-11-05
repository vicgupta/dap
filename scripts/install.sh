#!/bin/sh
# Install the DAP CLI into the current directory on macOS or Linux.
#
#   curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
#   sh install.sh
#   ./da run
#
# Does not change PATH and does not use sudo.
# Downloads use a temp directory inside the install folder and are removed.
# The only file left behind is da. Data stays in ./.dap when you run da.
# Set DAP_INSTALL_DIR, or pass --dir DIR, to install somewhere else.
set -eu

REPO="${DAP_REPO:-vicgupta/dap}"
# Newest GitHub release, including pre-releases (/releases/latest skips those).
# FALLBACK_TAG is used when that API does not answer. DAP_TAG overrides both.
FALLBACK_TAG="${DAP_TAG:-v0.1.0-alpha.4}"
resolve_tag() {
  if [ -n "${DAP_TAG-}" ]; then
    printf '%s\n' "$DAP_TAG"
    return
  fi
  body=$(curl -fsSL -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/${REPO}/releases?per_page=1" || true)
  tag=$(printf '%s\n' "$body" | awk -F'"' '/"tag_name"/ { print $4; exit }')
  if [ -z "$tag" ]; then
    tag=$FALLBACK_TAG
  fi
  printf '%s\n' "$tag"
}
TAG=$(resolve_tag)
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

# Temp dir inside the install folder. Removed on exit, including failure.
# Empty .dap/tmp and .dap parents are removed so a fresh folder keeps only da.
workdir="${dest}/.dap/tmp/install.$$"
cleanup() {
  rm -rf "$workdir"
  rmdir "${dest}/.dap/tmp" 2>/dev/null || true
  rmdir "${dest}/.dap" 2>/dev/null || true
}
trap cleanup EXIT
mkdir -p "$workdir"

missing_linux_asset() {
  echo "Release ${TAG} has no matching Linux asset (${cli_tar})." >&2
  echo "That release does not include a Linux build of da. Set DAP_TAG to a release that publishes da-*-linux-*.tar.gz." >&2
  exit 1
}

base="https://github.com/${REPO}/releases/download/${TAG}"
echo "Downloading ${TAG} ($(uname -s) ${goarch}) from ${base}"
if ! curl -fsSL -o "$workdir/checksums.txt" "${base}/checksums.txt"; then
  echo "Could not download checksums.txt from ${TAG}." >&2
  exit 1
fi

expected=$(awk -v f="$cli_tar" '$2 == f { print $1; exit }' "$workdir/checksums.txt")
if [ -z "$expected" ]; then
  if [ "$os" = "Linux" ]; then
    missing_linux_asset
  fi
  echo "No SHA256 line for ${cli_tar} in checksums.txt" >&2
  exit 1
fi

if ! curl -fsSL -o "$workdir/$cli_tar" "${base}/${cli_tar}"; then
  if [ "$os" = "Linux" ]; then
    missing_linux_asset
  fi
  echo "Could not download ${cli_tar} from ${TAG}." >&2
  exit 1
fi
if [ ! -f "$workdir/$cli_tar" ]; then
  if [ "$os" = "Linux" ]; then
    missing_linux_asset
  fi
  echo "Could not download ${cli_tar} from ${TAG}." >&2
  exit 1
fi

if [ "$os" = "Darwin" ]; then
  if ! command -v shasum >/dev/null 2>&1; then
    echo "shasum is required to verify the download on macOS." >&2
    exit 1
  fi
  got=$(shasum -a 256 "$workdir/$cli_tar" | awk '{ print $1 }')
else
  if ! command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum is required to verify the download on Linux." >&2
    exit 1
  fi
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
if [ "$os" = "Linux" ]; then
  echo "On a machine with no desktop or browser, run ./da run --no-browser and open http://127.0.0.1:7391."
fi
