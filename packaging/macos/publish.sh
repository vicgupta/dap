#!/bin/bash
# Upload dist/release/ to a GitHub release for the given tag.
# Creates a pre-release when the tag contains -alpha, -beta, or -rc.
# Uses the current gh login (GH_TOKEN in GitHub Actions). No token is written to disk.
#
# Usage: packaging/macos/publish.sh v0.1.0-alpha.2
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
tag="${1:-}"
if [ -z "$tag" ]; then
  echo "usage: packaging/macos/publish.sh v0.1.0-alpha.1" >&2
  exit 1
fi
case "$tag" in
  v*) ;;
  *) tag="v${tag}" ;;
esac

out="$root/dist/release"
notes="$root/dist/NOTES.md"
if [ ! -d "$out" ] || [ ! -f "$notes" ]; then
  echo "missing ${out}; run packaging/release.sh first" >&2
  exit 1
fi

# shellcheck source=packaging/cli_assets.sh
source "$root/packaging/cli_assets.sh"
cli_release_assert_clean "$out"
cli_release_require "$out" checksums
assets=()
while IFS= read -r asset; do
  [ -n "$asset" ] || continue
  assets+=("$asset")
done < <(cli_release_list "$out")

cd "$root"
if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" "${assets[@]}" --clobber
  exit 0
fi

args=(--title "$tag" --notes-file "$notes")
case "$tag" in
  *-alpha*|*-beta*|*-rc*) args+=(--prerelease) ;;
esac
gh release create "$tag" "${args[@]}" "${assets[@]}"
