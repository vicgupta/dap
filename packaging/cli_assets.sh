#!/bin/bash
# Release uploads are CLI binaries only.
# Keep this list in sync with packaging/release.sh and packaging/macos/publish.sh.
#
#   da-<version>-darwin-{arm64,amd64,universal}.tar.gz
#   da-<version>-linux-{amd64,arm64}.tar.gz
#   da-<version>-windows-{amd64,arm64}.zip
#   checksums.txt
#   install.sh
#   install.ps1

cli_release_keep() {
  case "$1" in
    da-*-darwin-*.tar.gz|da-*-linux-*.tar.gz|da-*-windows-*.zip|install.sh|install.ps1|checksums.txt)
      return 0
      ;;
  esac
  return 1
}

# Delete anything in $1 that is not a CLI release asset.
cli_release_prune() {
  local out="$1"
  local name base
  for name in "$out"/*; do
    [ -f "$name" ] || continue
    base=$(basename "$name")
    if cli_release_keep "$base"; then
      continue
    fi
    echo "omitting ${base} from the release (CLI binaries only)" >&2
    rm -f "$name"
  done
}

# Fail if $1 still contains a non-CLI file.
cli_release_assert_clean() {
  local out="$1"
  local name base bad=0
  for name in "$out"/*; do
    [ -e "$name" ] || continue
    base=$(basename "$name")
    if [ -f "$name" ] && cli_release_keep "$base"; then
      continue
    fi
    echo "refusing to upload ${base}; releases are CLI binaries only" >&2
    bad=1
  done
  [ "$bad" -eq 0 ]
}

# Fail unless every required asset is present.
# Pass "checksums" as $2 after checksums.txt has been written.
cli_release_require() {
  local out="$1"
  local with_checksums="${2:-}"
  local pattern missing=0
  local patterns=(
    'da-*-darwin-arm64.tar.gz'
    'da-*-darwin-amd64.tar.gz'
    'da-*-darwin-universal.tar.gz'
    'da-*-linux-amd64.tar.gz'
    'da-*-linux-arm64.tar.gz'
    'da-*-windows-amd64.zip'
    'da-*-windows-arm64.zip'
    'install.sh'
    'install.ps1'
  )
  if [ "$with_checksums" = "checksums" ]; then
    patterns+=('checksums.txt')
  fi
  local shopt_state
  shopt_state=$(shopt -p nullglob || true)
  shopt -s nullglob
  for pattern in "${patterns[@]}"; do
    local matches=( "$out"/$pattern )
    if [ "${#matches[@]}" -eq 0 ]; then
      echo "release is missing ${pattern}" >&2
      missing=1
    fi
  done
  eval "$shopt_state"
  [ "$missing" -eq 0 ]
}

# Print the absolute paths to upload, one per line.
cli_release_list() {
  local out="$1"
  local pattern f
  local shopt_state
  shopt_state=$(shopt -p nullglob || true)
  shopt -s nullglob
  for pattern in \
    'da-*-darwin-*.tar.gz' \
    'da-*-linux-*.tar.gz' \
    'da-*-windows-*.zip' \
    'checksums.txt' \
    'install.sh' \
    'install.ps1'
  do
    for f in "$out"/$pattern; do
      printf '%s\n' "$f"
    done
  done
  eval "$shopt_state"
}
