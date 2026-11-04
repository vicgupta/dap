#!/bin/bash
# Build macOS, Linux, and Windows release artifacts into dist/release/.
# Does not upload. packaging/macos/publish.sh uploads with the current gh login.
#
# Usage: packaging/release.sh v0.1.0-alpha.2
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"

version="${1:-}"
version="${version#v}"
if [ -z "$version" ]; then
  echo "usage: packaging/release.sh v0.1.0-alpha.2" >&2
  exit 1
fi
tag="v${version}"

rm -rf "$root/dist/work" "$root/dist/release"
mkdir -p "$root/dist/release"

export DAP_RELEASE_APPEND=1
packaging/macos/release.sh "$tag"
packaging/linux/package.sh "$tag"
packaging/windows/package.sh "$tag"

out="$root/dist/release"
awk -v tag="$tag" '
  index($0, "DAP_TAG:-") {
    print "TAG=\"${DAP_TAG:-" tag "}\""
    next
  }
  { print }
' "$root/scripts/install.sh" > "$out/install.sh"
chmod 755 "$out/install.sh"

awk -v tag="$tag" '
  index($0, "$env:DAP_TAG") && index($0, "else") {
    print "$Tag = if ($env:DAP_TAG) { $env:DAP_TAG } else { \"" tag "\" }"
    next
  }
  { print }
' "$root/scripts/install.ps1" > "$out/install.ps1"

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
DAP \`${tag}\` for macOS, Linux, and Windows.
macOS builds are ad-hoc signed and not notarized. Linux and Windows builds are unsigned.

macOS or Linux:

\`\`\`bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.sh
sh install.sh
./da run
\`\`\`

Windows (PowerShell):

\`\`\`powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/vicgupta/dap/main/scripts/install.ps1 -OutFile .\\install.ps1
powershell -ExecutionPolicy Bypass -Scope Process -File .\\install.ps1
.\\da.exe run
\`\`\`

The installers check SHA256 and copy \`da\` or \`da.exe\` into the current directory. They do not change PATH and do not need sudo or an administrator prompt. Data stays in ./.dap in that folder. A relative \$DAP_HOME is resolved from the folder.

macOS Gatekeeper: right-click DAP.app > Open, or System Settings > Privacy & Security > Open Anyway, or \`xattr -dr com.apple.quarantine DAP.app\`.

Windows SmartScreen: More info > Run anyway. \`install.ps1\` also unblocks the installed copy.

da run opens the URL in the default browser (\`open\` on macOS, \`rundll32 url.dll,FileProtocolHandler\` on Windows, \`xdg-open\` on Linux). Chrome is not required. Example plugins need Python 3. Gmail and Outlook need an OAuth client id. See docs/release.md.
EOF

echo "Artifacts in ${out}"
ls -lh "$out"
echo "--- checksums ---"
cat "$out/checksums.txt"
