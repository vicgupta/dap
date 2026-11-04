#!/bin/bash
# Build the macOS artifacts into dist/release/.
# Ad-hoc signs with rcodesign on Linux, or codesign -s - on macOS.
# Does not notarize and does not upload.
#
# Usage: packaging/macos/release.sh v0.1.0-alpha.1
# The all-platform entry point is packaging/release.sh. When
# DAP_RELEASE_APPEND=1, this script only adds macOS files and leaves
# checksums and installers to the caller.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

version="${1:-}"
version="${version#v}"
if [ -z "$version" ]; then
  echo "usage: packaging/macos/release.sh v0.1.0-alpha.1" >&2
  exit 1
fi
tag="v${version}"
bundle_version="${version%%-*}"

find_lipo() {
  for c in lipo llvm-lipo llvm-lipo-18 llvm-lipo-19 llvm-lipo-20; do
    if command -v "$c" >/dev/null 2>&1; then
      command -v "$c"
      return 0
    fi
  done
  if [ -x /usr/lib/llvm-18/bin/llvm-lipo ]; then
    echo /usr/lib/llvm-18/bin/llvm-lipo
    return 0
  fi
  echo "lipo not found. On Ubuntu install the llvm package." >&2
  return 1
}

sign_path() {
  if command -v rcodesign >/dev/null 2>&1; then
    rcodesign sign "$1"
    return
  fi
  if command -v codesign >/dev/null 2>&1; then
    if [ -d "$1" ]; then
      codesign --force --sign - "$1/Contents/MacOS/da"
      codesign --force --sign - "$1/Contents/MacOS/DAP"
      codesign --force --sign - "$1"
    else
      codesign --force --sign - "$1"
    fi
    return
  fi
  echo "Need rcodesign (Linux) or codesign (macOS) to ad-hoc sign." >&2
  exit 1
}

lipo_bin=$(find_lipo)
commit=$(git -C "$root" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X github.com/vicgupta/dap/internal/version.Release=${version} -X github.com/vicgupta/dap/internal/version.Commit=${commit} -X github.com/vicgupta/dap/internal/version.Date=${build_date}"

work="$root/dist/work/macos"
out="$root/dist/release"
if [ "${DAP_RELEASE_APPEND:-}" = 1 ]; then
  rm -rf "$work"
else
  rm -rf "$root/dist/work" "$out"
fi
mkdir -p "$work/bin" "$out"

echo "Building da ${version} (${commit})"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$ldflags" -o "$work/bin/da-arm64" ./cmd/da
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$work/bin/da-amd64" ./cmd/da
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o "$work/bin/DAP-arm64" ./packaging/macos/launcher
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$work/bin/DAP-amd64" ./packaging/macos/launcher
"$lipo_bin" -create -output "$work/bin/da-universal" "$work/bin/da-arm64" "$work/bin/da-amd64"
"$lipo_bin" -create -output "$work/bin/DAP-universal" "$work/bin/DAP-arm64" "$work/bin/DAP-amd64"
"$lipo_bin" -info "$work/bin/da-universal" "$work/bin/DAP-universal"

go run ./packaging/macos/icns "$work/AppIcon.icns"

cat > "$work/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>DAP</string>
  <key>CFBundleExecutable</key>
  <string>DAP</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundleIdentifier</key>
  <string>com.vicgupta.dap</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>DAP</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${version}</string>
  <key>CFBundleVersion</key>
  <string>${bundle_version}</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

cat > "$work/INSTALL.txt" << EOF
DAP ${version} for macOS
===========================

This zip contains DAP.app. Double-clicking it runs \`da run\` and opens the
local UI. Your data stays in ./.dap in the working directory (or \$DAP_HOME if you set it). The da
command-line binary is inside the app:

  DAP.app/Contents/MacOS/da

A separate CLI tarball is also on the GitHub release.

First launch (Gatekeeper)
-------------------------
This build is ad-hoc signed. It is not notarized, so macOS will block the
first open after download.

Pick one:

1. Right-click DAP.app and choose Open, then Open again.
2. System Settings > Privacy & Security, then Open Anyway next to DAP.
3. In Terminal, from the folder that contains DAP.app:

     xattr -dr com.apple.quarantine DAP.app

da run opens the URL in the default browser. Chrome is not required.
da run --no-browser only prints the URL.

Plugins
-------
Example plugins are in DAP.app/Contents/Resources/plugins. They need
Python 3 (python3 on PATH). From Terminal:

  da plugin install /Applications/DAP.app/Contents/Resources/plugins/hello-watch --approve
  da plugin install /Applications/DAP.app/Contents/Resources/plugins/workflow --approve
  da plugin install /Applications/DAP.app/Contents/Resources/plugins/mail-fake --approve
  da plugin install /Applications/DAP.app/Contents/Resources/plugins/mail-gmail --approve
  da plugin install /Applications/DAP.app/Contents/Resources/plugins/mail-graph --approve
  da plugin install /Applications/DAP.app/Contents/Resources/plugins/inbox-followups --approve

If you installed the app to ~/Applications, use that path instead.
Gmail and Outlook still need an OAuth client id on the Connections page.
The demo mailbox (mail-fake) does not.
EOF

assemble() {
  local arch="$1"
  local da_bin="$2"
  local launch_bin="$3"
  local stage="$work/stage-$arch"
  local app="$stage/DAP.app"
  rm -rf "$stage"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/plugins"
  cp "$work/Info.plist" "$app/Contents/Info.plist"
  printf 'APPL????' > "$app/Contents/PkgInfo"
  cp "$da_bin" "$app/Contents/MacOS/da"
  cp "$launch_bin" "$app/Contents/MacOS/DAP"
  chmod 755 "$app/Contents/MacOS/da" "$app/Contents/MacOS/DAP"
  cp "$work/AppIcon.icns" "$app/Contents/Resources/AppIcon.icns"
  cp -a "$root/plugins/." "$app/Contents/Resources/plugins/"
  cp "$root/LICENSE" "$stage/LICENSE"
  cp "$work/INSTALL.txt" "$stage/INSTALL.txt"
  echo "Signing DAP.app (${arch})"
  sign_path "$app"
  (cd "$stage" && zip -r -X "$out/DAP-${version}-macos-${arch}.zip" DAP.app INSTALL.txt LICENSE)
}

assemble arm64 "$work/bin/da-arm64" "$work/bin/DAP-arm64"
assemble amd64 "$work/bin/da-amd64" "$work/bin/DAP-amd64"
assemble universal "$work/bin/da-universal" "$work/bin/DAP-universal"

for arch in arm64 amd64 universal; do
  src="$work/bin/da-${arch}"
  tmp=$(mktemp -d)
  cp "$src" "$tmp/da"
  chmod 755 "$tmp/da"
  sign_path "$tmp/da"
  tar -C "$tmp" -czf "$out/da-${version}-darwin-${arch}.tar.gz" da
  rm -rf "$tmp"
done

if [ "${DAP_RELEASE_APPEND:-}" = 1 ]; then
  echo "macOS artifacts added to ${out}"
  exit 0
fi

awk -v tag="$tag" '
  index($0, "DAP_TAG:-") {
    print "TAG=\"${DAP_TAG:-" tag "}\""
    next
  }
  { print }
' "$root/scripts/install.sh" > "$out/install.sh"
chmod 755 "$out/install.sh"

(
  cd "$out"
  for name in *; do
    [ -f "$name" ] || continue
    [ "$name" = checksums.txt ] && continue
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$name"
    else
      shasum -a 256 "$name"
    fi
  done
) | sort -k2 > "$out/checksums.txt"

cat > "$root/dist/NOTES.md" << EOF
macOS pre-release of DAP (\`${tag}\`). Ad-hoc signed, not notarized.

\`\`\`bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
sh install.sh
./da run
\`\`\`

\`install.sh\` checks SHA256, copies \`da\` into the current directory, and clears the quarantine flag on that binary. It does not change PATH. Data stays in ./.dap in that folder. A relative \$DAP_HOME is resolved from the folder.

Direct downloads are on this release. Apple Silicon: \`DAP-${version}-macos-arm64.zip\` and \`da-${version}-darwin-arm64.tar.gz\`. Intel: the amd64 files. Universal: \`DAP-${version}-macos-universal.zip\` and \`da-${version}-darwin-universal.tar.gz\`. Checksums are in \`checksums.txt\`.

Gatekeeper, if macOS blocks the first open: right-click DAP.app > Open; or System Settings > Privacy & Security > Open Anyway; or \`xattr -dr com.apple.quarantine DAP.app\`.

da run opens the URL in the default browser. Chrome is not required. Example plugins need Python 3. Gmail and Outlook need an OAuth client id. See docs/release.md.
EOF

echo "Artifacts in ${out}"
ls -lh "$out"
echo "--- checksums ---"
cat "$out/checksums.txt"
