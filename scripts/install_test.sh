#!/bin/sh
# Hermetic checks for scripts/install.sh. No network and no writes outside the temp root.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
installer="$root/scripts/install.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# The script says it does not use sudo. A real sudo command is still a failure.
if grep -nE '(^|[[:space:]])sudo([[:space:]]|$)' "$installer" >/dev/null; then
  fail "install.sh must not use sudo"
fi
if grep -nE '(^|[[:space:]])gh[[:space:]]' "$installer" >/dev/null; then
  fail "install.sh must not call the gh CLI"
fi
if ! grep -n 'https://api.github.com/repos/${REPO}/releases?per_page=1' "$installer" >/dev/null; then
  fail "install.sh must look up the newest release on the public GitHub API"
fi

sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
bindir="$sandbox/bin"
mkdir -p "$bindir"

cat > "$bindir/uname" << 'EOF'
#!/bin/sh
case "${1:-}" in
  -s) printf '%s\n' "${FAKE_UNAME_S:?}" ;;
  -m) printf '%s\n' "${FAKE_UNAME_M:?}" ;;
  *)
    echo "unexpected uname $*" >&2
    exit 1
    ;;
esac
EOF

cat > "$bindir/curl" << 'EOF'
#!/bin/sh
out=""
url=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then
    out=$arg
  fi
  case "$arg" in
    http://*|https://*) url=$arg ;;
  esac
  prev=$arg
done
printf '%s\n' "$url" >> "${CURL_LOG:?}"
case "$url" in
  *api.github.com*)
    if [ "${FAIL_API:-}" = 1 ]; then
      exit 22
    fi
    printf '%s\n' "{\"tag_name\":\"${API_TAG:-v9.9.9-test}\"}"
    exit 0
    ;;
esac
file=$(basename "$url")
if [ "${FAIL_TAR:-}" = 1 ]; then
  case "$file" in
    da-*) exit 22 ;;
  esac
fi
src="${FIXTURE_DIR:?}/$file"
if [ ! -f "$src" ]; then
  exit 22
fi
if [ -n "$out" ]; then
  cp "$src" "$out"
else
  cat "$src"
fi
EOF

cat > "$bindir/shasum" << 'EOF'
#!/bin/sh
if [ "${BLOCK_SHASUM:-}" = 1 ]; then
  echo "shasum should not run on Linux" >&2
  exit 1
fi
file=""
for arg in "$@"; do
  file=$arg
done
exec /usr/bin/sha256sum "$file"
EOF

cat > "$bindir/sha256sum" << 'EOF'
#!/bin/sh
if [ "${BLOCK_SHA256SUM:-}" = 1 ]; then
  echo "sha256sum should not run on macOS" >&2
  exit 1
fi
exec /usr/bin/sha256sum "$@"
EOF

chmod 755 "$bindir/uname" "$bindir/curl" "$bindir/shasum" "$bindir/sha256sum"

make_fixture() {
  # $1 version without v, $2 os (linux|darwin), $3 arch, $4 fixture dir
  ver=$1
  osname=$2
  arch=$3
  dir=$4
  mkdir -p "$dir"
  stage=$(mktemp -d)
  printf '#!/bin/sh\necho da-%s-%s\n' "$osname" "$arch" > "$stage/da"
  chmod 755 "$stage/da"
  mkdir -p "$stage/plugins"
  echo extra > "$stage/plugins/extra.txt"
  echo launcher > "$stage/dap"
  name="da-${ver}-${osname}-${arch}.tar.gz"
  tar -C "$stage" -czf "$dir/$name" da dap plugins
  rm -rf "$stage"
  (
    cd "$dir"
    sha256sum "$name" > checksums.txt
  )
}

run_install() {
  # Uses the caller's env (FAKE_UNAME_*, DAP_TAG, FIXTURE_DIR, BLOCK_*).
  dest=$1
  logs=$2
  mkdir -p "$dest"
  : > "$logs/curl.log"
  set +e
  (
    cd "$dest"
    export HOME="$sandbox/home"
    export PATH="$bindir:$PATH"
    export CURL_LOG="$logs/curl.log"
    export FAKE_UNAME_S FAKE_UNAME_M FIXTURE_DIR
    [ -n "${BLOCK_SHASUM+x}" ] && export BLOCK_SHASUM
    [ -n "${BLOCK_SHA256SUM+x}" ] && export BLOCK_SHA256SUM
    [ -n "${FAIL_API+x}" ] && export FAIL_API
    [ -n "${FAIL_TAR+x}" ] && export FAIL_TAR
    [ -n "${API_TAG+x}" ] && export API_TAG
    if [ -n "${DAP_TAG+x}" ]; then
      export DAP_TAG
    else
      unset DAP_TAG
    fi
    sh "$installer"
  ) > "$logs/out" 2> "$logs/err"
  code=$?
  set -e
  return "$code"
}

assert_only_da() {
  dest=$1
  found=$(find "$dest" -mindepth 1 -print | sort)
  if [ "$found" != "$dest/da" ]; then
    fail "install wrote more than da in $dest:
$found"
  fi
  if [ ! -x "$dest/da" ]; then
    fail "da is not executable"
  fi
  home_files=$(find "$sandbox/home" -mindepth 1 -print 2>/dev/null || true)
  if [ -n "$home_files" ]; then
    fail "install wrote outside the folder, into HOME:
$home_files"
  fi
}

mkdir -p "$sandbox/home"

# Linux x86_64: sha256sum, amd64 asset, only da left behind.
fix="$sandbox/fix-linux-amd64"
make_fixture "1.2.3-test" linux amd64 "$fix"
dest="$sandbox/linux-amd64"
logs="$sandbox/logs-linux-amd64"
mkdir -p "$logs"
if ! (
  FAKE_UNAME_S=Linux
  FAKE_UNAME_M=x86_64
  DAP_TAG=v1.2.3-test
  FIXTURE_DIR="$fix"
  BLOCK_SHASUM=1
  API_TAG=v0.0.0-ignored
  run_install "$dest" "$logs"
); then
  echo "---- stdout ----" >&2
  cat "$logs/out" >&2
  echo "---- stderr ----" >&2
  cat "$logs/err" >&2
  fail "linux amd64 install failed"
fi
assert_only_da "$dest"
got=$("$dest/da")
if [ "$got" != "da-linux-amd64" ]; then
  fail "installed the wrong binary: $got"
fi
if ! grep -q 'da-1.2.3-test-linux-amd64.tar.gz' "$logs/curl.log"; then
  fail "did not download the linux amd64 tarball"
fi
if grep -q 'releases?per_page=1' "$logs/curl.log"; then
  fail "DAP_TAG should skip the releases API"
fi
if ! grep -q 'no desktop or browser' "$logs/out"; then
  fail "linux install did not mention --no-browser"
fi
echo "ok  linux amd64"

# aarch64 and arm64 both select the arm64 asset.
for machine in aarch64 arm64; do
  fix="$sandbox/fix-linux-arm64"
  if [ ! -d "$fix" ]; then
    make_fixture "1.2.3-test" linux arm64 "$fix"
  fi
  dest="$sandbox/linux-$machine"
  logs="$sandbox/logs-linux-$machine"
  mkdir -p "$logs"
  if ! (
    FAKE_UNAME_S=Linux
    FAKE_UNAME_M=$machine
    DAP_TAG=v1.2.3-test
    FIXTURE_DIR="$fix"
    BLOCK_SHASUM=1
    run_install "$dest" "$logs"
  ); then
    cat "$logs/err" >&2
    fail "linux $machine install failed"
  fi
  assert_only_da "$dest"
  if [ "$("$dest/da")" != "da-linux-arm64" ]; then
    fail "linux $machine installed the wrong binary"
  fi
  echo "ok  linux $machine"
done

# macOS uses shasum and the darwin tarball, even if sha256sum is broken.
fix="$sandbox/fix-darwin"
make_fixture "1.2.3-test" darwin arm64 "$fix"
dest="$sandbox/darwin"
logs="$sandbox/logs-darwin"
mkdir -p "$logs"
if ! (
  FAKE_UNAME_S=Darwin
  FAKE_UNAME_M=arm64
  DAP_TAG=v1.2.3-test
  FIXTURE_DIR="$fix"
  BLOCK_SHA256SUM=1
  run_install "$dest" "$logs"
); then
  cat "$logs/err" >&2
  fail "darwin install failed"
fi
assert_only_da "$dest"
if [ "$("$dest/da")" != "da-darwin-arm64" ]; then
  fail "darwin installed the wrong binary"
fi
if ! grep -q 'da-1.2.3-test-darwin-arm64.tar.gz' "$logs/curl.log"; then
  fail "did not download the darwin tarball"
fi
echo "ok  darwin arm64"

# Selected release has checksums and no Linux asset.
fix="$sandbox/fix-missing"
mkdir -p "$fix"
printf '%s\n' "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  da-0.1.0-alpha.4-darwin-arm64.tar.gz" > "$fix/checksums.txt"
dest="$sandbox/missing"
logs="$sandbox/logs-missing"
mkdir -p "$logs"
set +e
(
  FAKE_UNAME_S=Linux
  FAKE_UNAME_M=x86_64
  DAP_TAG=v0.1.0-alpha.4
  FIXTURE_DIR="$fix"
  BLOCK_SHASUM=1
  run_install "$dest" "$logs"
)
code=$?
set -e
if [ "$code" -eq 0 ]; then
  fail "missing linux asset should fail"
fi
if ! grep -q 'Release v0.1.0-alpha.4 has no matching Linux asset (da-0.1.0-alpha.4-linux-amd64.tar.gz).' "$logs/err"; then
  echo "---- stderr ----" >&2
  cat "$logs/err" >&2
  fail "missing linux asset error is not clear"
fi
if [ -e "$dest/da" ]; then
  fail "failed install still wrote da"
fi
left=$(find "$dest" -mindepth 1 -print)
if [ -n "$left" ]; then
  fail "failed install left files behind:
$left"
fi
echo "ok  missing linux asset"

# API failure falls back to v0.1.0-alpha.4, which has no Linux asset.
dest="$sandbox/fallback"
logs="$sandbox/logs-fallback"
mkdir -p "$logs"
set +e
(
  FAKE_UNAME_S=Linux
  FAKE_UNAME_M=aarch64
  FIXTURE_DIR="$fix"
  FAIL_API=1
  BLOCK_SHASUM=1
  unset DAP_TAG
  run_install "$dest" "$logs"
)
code=$?
set -e
if [ "$code" -eq 0 ]; then
  fail "fallback tag should fail on Linux"
fi
if ! grep -q 'Release v0.1.0-alpha.4 has no matching Linux asset (da-0.1.0-alpha.4-linux-arm64.tar.gz).' "$logs/err"; then
  cat "$logs/err" >&2
  fail "fallback did not report the alpha.4 linux asset"
fi
if ! grep -q 'releases?per_page=1' "$logs/curl.log"; then
  fail "fallback path did not call the releases API"
fi
left=$(find "$dest" -mindepth 1 -print)
if [ -n "$left" ]; then
  fail "fallback install left files behind:
$left"
fi
echo "ok  fallback tag"

# Unsupported CPU.
dest="$sandbox/cpu"
logs="$sandbox/logs-cpu"
mkdir -p "$logs"
set +e
(
  FAKE_UNAME_S=Linux
  FAKE_UNAME_M=i686
  DAP_TAG=v1.2.3-test
  FIXTURE_DIR="$fix"
  run_install "$dest" "$logs"
)
code=$?
set -e
if [ "$code" -eq 0 ]; then
  fail "unsupported CPU should fail"
fi
if ! grep -q 'Unsupported CPU: i686' "$logs/err"; then
  fail "unsupported CPU error missing"
fi
echo "ok  unsupported cpu"

echo "ok  install.sh"
