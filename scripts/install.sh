#!/bin/sh
# Install a DAP release from the private GitHub repo.
# One-time setup: gh auth login
# macOS or Linux:
#   gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern install.sh && sh install.sh
# v0.1.0-alpha.1 ships macOS packages only. Later tags also ship Linux archives.
set -eu

REPO="${DAP_REPO:-vicgupta/dap}"
TAG="${DAP_TAG:-v0.1.0-alpha.1}"
VER="${TAG#v}"

if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI (gh) is required. Install it, then run: gh auth login" >&2
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "gh is not logged in. Run: gh auth login" >&2
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
  app_zip="DAP-${VER}-macos-${goarch}.zip"
  cli_tar="da-${VER}-darwin-${goarch}.tar.gz"
else
  app_zip=""
  cli_tar="da-${VER}-linux-${goarch}.tar.gz"
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

echo "Downloading ${TAG} ($(uname -s) ${goarch}) from ${REPO}"
if [ "$os" = "Darwin" ]; then
  if ! gh release download "$TAG" --repo "$REPO" \
    --dir "$workdir" \
    --pattern checksums.txt \
    --pattern "$app_zip" \
    --pattern "$cli_tar"; then
    echo "Could not download the macOS packages for ${TAG}." >&2
    exit 1
  fi
else
  if ! gh release download "$TAG" --repo "$REPO" \
    --dir "$workdir" \
    --pattern checksums.txt \
    --pattern "$cli_tar"; then
    echo "Could not download ${cli_tar} from ${TAG}." >&2
    echo "v0.1.0-alpha.1 includes macOS packages only. Linux archives are built for a later tag." >&2
    exit 1
  fi
  if [ ! -f "$workdir/$cli_tar" ]; then
    echo "Could not download ${cli_tar} from ${TAG}." >&2
    echo "v0.1.0-alpha.1 includes macOS packages only. Linux archives are built for a later tag." >&2
    exit 1
  fi
fi

verify() {
  file=$1
  expected=$(awk -v f="$file" '$2 == f { print $1; exit }' "$workdir/checksums.txt")
  if [ -z "$expected" ]; then
    echo "No SHA256 line for ${file} in checksums.txt" >&2
    exit 1
  fi
  if command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$workdir/$file" | awk '{ print $1 }')
  else
    got=$(sha256sum "$workdir/$file" | awk '{ print $1 }')
  fi
  if [ "$got" != "$expected" ]; then
    echo "SHA256 mismatch for ${file}" >&2
    echo "  expected ${expected}" >&2
    echo "  got      ${got}" >&2
    exit 1
  fi
  echo "SHA256 ok  ${file}"
}

if [ -n "$app_zip" ]; then
  verify "$app_zip"
fi
verify "$cli_tar"

bindir="${HOME}/.local/bin"
mkdir -p "$bindir"

if [ "$os" = "Darwin" ]; then
  if [ -w /Applications ]; then
    dest=/Applications
  else
    dest="${HOME}/Applications"
  fi
  mkdir -p "$dest"
  unzip -q "$workdir/$app_zip" -d "$workdir/unzip"
  if [ -e "$dest/DAP.app" ]; then
    rm -rf "$dest/DAP.app"
  fi
  mv "$workdir/unzip/DAP.app" "$dest/DAP.app"
  if command -v xattr >/dev/null 2>&1; then
    xattr -dr com.apple.quarantine "$dest/DAP.app" || true
  fi
  tar -xzf "$workdir/$cli_tar" -C "$workdir"
  install -m 755 "$workdir/da" "$bindir/da"
  if command -v xattr >/dev/null 2>&1; then
    xattr -d com.apple.quarantine "$bindir/da" 2>/dev/null || true
  fi
  echo
  echo "Installed DAP.app at ${dest}/DAP.app"
  echo "Installed da at ${bindir}/da"
  plugins="${dest}/DAP.app/Contents/Resources/plugins"
else
  tar -xzf "$workdir/$cli_tar" -C "$workdir"
  install -m 755 "$workdir/da" "$bindir/da"
  install -m 755 "$workdir/dap" "$bindir/dap"
  sharedir="${HOME}/.local/share/dap"
  mkdir -p "$sharedir"
  rm -rf "$sharedir/plugins"
  mv "$workdir/plugins" "$sharedir/plugins"
  echo
  echo "Installed da and dap at ${bindir}"
  echo "Installed plugins at ${sharedir}/plugins"
  plugins="${sharedir}/plugins"
fi

case ":$PATH:" in
  *":${bindir}:"*) ;;
  *) echo "Add ${bindir} to PATH if your shell does not already include it:"
     echo "  export PATH=\"${bindir}:\$PATH\""
     ;;
esac
echo
if [ "$os" = "Darwin" ]; then
  echo "Double-click DAP.app to run the UI (same as: da run)."
else
  echo "Run dap to open the UI (same as: da run)."
fi
echo "Data directory: \${DAP_HOME:-\$HOME/.dap}"
echo
if [ "$os" = "Darwin" ]; then
  echo "If macOS still blocks the app:"
  echo "  right-click DAP.app > Open, then Open"
  echo "  or System Settings > Privacy & Security > Open Anyway"
  echo "  or: xattr -dr com.apple.quarantine \"${dest}/DAP.app\""
  echo
fi
echo "Example plugins need python3:"
echo "  ${bindir}/da plugin install \"${plugins}/mail-fake\" --approve"
echo "  ${bindir}/da plugin install \"${plugins}/inbox-followups\" --approve"
echo "  ${bindir}/da plugin install \"${plugins}/hello-watch\" --approve"
echo "  ${bindir}/da plugin install \"${plugins}/workflow\" --approve"
echo "Gmail and Outlook also need a client id. See docs/inbox-followups.md in the repo."
