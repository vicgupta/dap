#!/bin/sh
# Install the DAP macOS pre-release from the private GitHub repo.
# One-time setup: gh auth login
# Then: gh release download v0.1.0-alpha.1 --repo vicgupta/dap --pattern install.sh && sh install.sh
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

if [ "$(uname -s)" != "Darwin" ]; then
  echo "v${VER} is a macOS build. This Mac is not the current machine (uname: $(uname -s))." >&2
  exit 1
fi

case "$(uname -m)" in
  arm64) goarch=arm64 ;;
  x86_64) goarch=amd64 ;;
  *)
    echo "Unsupported CPU: $(uname -m). Use the universal zip from the release page." >&2
    exit 1
    ;;
esac

app_zip="DAP-${VER}-macos-${goarch}.zip"
cli_tar="da-${VER}-darwin-${goarch}.tar.gz"

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

echo "Downloading ${TAG} (${goarch}) from ${REPO}"
gh release download "$TAG" --repo "$REPO" \
  --dir "$workdir" \
  --pattern checksums.txt \
  --pattern "$app_zip" \
  --pattern "$cli_tar"

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

verify "$app_zip"
verify "$cli_tar"

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

bindir="${HOME}/.local/bin"
mkdir -p "$bindir"
tar -xzf "$workdir/$cli_tar" -C "$workdir"
install -m 755 "$workdir/da" "$bindir/da"
if command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$bindir/da" 2>/dev/null || true
fi

echo
echo "Installed DAP.app at ${dest}/DAP.app"
echo "Installed da at ${bindir}/da"
case ":$PATH:" in
  *":${bindir}:"*) ;;
  *) echo "Add ${bindir} to PATH if your shell does not already include it:"
     echo "  export PATH=\"${bindir}:\$PATH\""
     ;;
esac
echo
echo "Double-click DAP.app to run the UI (same as: da run)."
echo "Data directory: \${DAP_HOME:-\$HOME/.dap}"
echo
echo "If macOS still blocks the app:"
echo "  right-click DAP.app > Open, then Open"
echo "  or System Settings > Privacy & Security > Open Anyway"
echo "  or: xattr -dr com.apple.quarantine \"${dest}/DAP.app\""
echo
echo "Example plugins ship inside the app and need python3:"
plugins="${dest}/DAP.app/Contents/Resources/plugins"
echo "  ${bindir}/da plugin install \"${plugins}/mail-fake\" --approve"
echo "  ${bindir}/da plugin install \"${plugins}/inbox-followups\" --approve"
echo "  ${bindir}/da plugin install \"${plugins}/hello-watch\" --approve"
echo "  ${bindir}/da plugin install \"${plugins}/workflow\" --approve"
echo "Gmail and Outlook also need a client id. See docs/inbox-followups.md in the repo."
