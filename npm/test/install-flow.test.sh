#!/bin/sh
set -eu
root=$(CDPATH='' cd -P "$(dirname "$0")/../.." && pwd -P)
installer="$root/scripts/install.sh"
temp=$(mktemp -d "${TMPDIR:-/tmp}/gasa-flow.XXXXXX")
failed=0
cleanup() { rm -rf "$temp"; }
trap cleanup EXIT HUP INT TERM
mkdir "$temp/bin"
cat > "$temp/bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in -m) printf '%s\n' x86_64;; *) printf '%s\n' Linux;; esac
EOF
chmod +x "$temp/bin/uname"
cat > "$temp/bin/curl" <<'EOF'
#!/bin/sh
headers= body= url= status=200 location= body_value=ok
while [ "$#" -gt 0 ]; do
  case "$1" in
    -D) headers=$2; shift 2;; -o) body=$2; shift 2;; -w) shift 2;;
    -*) shift;; http://*|https://*) url=$1; shift;; *) shift;;
  esac
done
printf '%s\n' "$url" >> "$FLOW_LOG"
path=${url#*://}; path=/${path#*/}
case "$path" in
  /r0) status=302; location='https://github.com/r1';;
  /r1) status=302; location='https://github.com/r2';;
  /r2) status=302; location='https://github.com/r3';;
  /r3) status=302; location='https://github.com/r4';;
  /r4) status=302; location='https://github.com/r5';;
  /s0) status=302; location='https://github.com/s1';;
  /s1) status=302; location='https://github.com/s2';;
  /s2) status=302; location='https://github.com/s3';;
  /s3) status=302; location='https://github.com/s4';;
  /s4) status=302; location='https://github.com/s5';;
  /s5) status=302; location='https://github.com/s6';;
  /external) status=302; location='https://evil.example/never';;
  /http) status=302; location='http://github.com/no';;
  /port) status=302; location='https://github.com:8443/no';;
  /case) status=302; location='https://GitHub.com/ok';;
  /scheme) status=302; location='HTTPS://GitHub.com/ok';;
  */releases/download/v1.2.3/checksums.txt)
    if [ "${FLOW_HANG:-0}" = 1 ]; then : > "$FLOW_HANG_MARKER"; sleep 10; fi
    if [ "${FLOW_CONFLICT_CHECKSUM:-0}" = 1 ]; then cp "$FLOW_FIXTURE_DIR/checksums-conflict.txt" "$body"
    elif [ "${FLOW_OVERSIZE:-0}" = 1 ]; then cp "$FLOW_FIXTURE_DIR/checksums-large.txt" "$body"
    elif [ "${FLOW_BAD_CHECKSUM:-0}" = 1 ]; then cp "$FLOW_FIXTURE_DIR/checksums-bad.txt" "$body"
    else cp "$FLOW_FIXTURE_DIR/checksums.txt" "$body"; fi
    ;;
  */releases/download/v1.2.3/gasa_1.2.3_linux_amd64.tar.gz)
    if [ "${FLOW_OVERSIZE:-0}" = 1 ]; then cp "$FLOW_FIXTURE_DIR/archive-large.tar.gz" "$body"; else cp "$FLOW_FIXTURE_DIR/archive.tar.gz" "$body"; fi
    ;;
esac
case "$path" in */releases/download/*) printf 'HTTP/1.1 200 OK\r\n\r\n' > "$headers";; /r1|/r3|/r5|/case) printf 'HTTP/1.1 %s\r\nlocation: %s\r\n\r\n' "$status" "$location" > "$headers";;
  *) printf 'HTTP/1.1 %s\r\nLocation: %s\r\n\r\n' "$status" "$location" > "$headers";; esac
if [ "$status" = 200 ] && [ ! -f "$body" ]; then printf '%s' "$body_value" > "$body"; fi
printf '%s' "$status"
EOF
chmod +x "$temp/bin/curl"
export FLOW_LOG="$temp/requests.log"
check_download() {
  name=$1 route=$2 expected=$3
  : > "$FLOW_LOG"; out="$temp/$name.out"
  set +e
  PATH="$temp/bin:$PATH" sh "$installer" --test-download "https://github.com/$route" "$out" >/dev/null 2>&1
  code=$?
  set -e
  if [ "$expected" = pass ] && [ "$code" -eq 0 ] && [ "$(cat "$out")" = ok ]; then printf 'PASS: %s\n' "$name"
  elif [ "$expected" = fail ] && [ "$code" -ne 0 ]; then printf 'PASS: %s\n' "$name"
  else printf 'FAIL: %s (exit=%s)\n' "$name" "$code"; failed=1; fi
}
check_download 'mixed-case headers and five redirects' r0 pass
check_download 'sixth redirect rejected' s0 fail
check_download 'external host rejected before request' external fail
if ! grep -Fq 'evil.example' "$FLOW_LOG"; then printf 'PASS: forbidden host was not requested\n'; else printf 'FAIL: forbidden host was requested\n'; failed=1; fi
check_download 'HTTP redirect rejected' http fail
check_download 'non-443 port rejected' port fail
check_download 'uppercase GitHub host accepted' case pass
check_download 'uppercase HTTPS Location scheme accepted' scheme pass
for version in v1.2.3-. v1.2.3-.. v1.2.3-alpha.; do
  set +e
  PATH="$temp/bin:$PATH" HOME="$temp/home" GASA_VERSION="$version" GASA_INSTALL_DIR="$temp/version/gasa/bin" sh "$installer" > "$temp/version.out" 2>&1
  version_status=$?
  set -e
  if [ "$version_status" -ne 0 ] && grep -Fq 'Invalid version' "$temp/version.out"; then printf 'PASS: malformed version %s rejected\n' "$version"
  else printf 'FAIL: malformed version %s was not rejected\n' "$version"; failed=1; fi
done
mkdir "$temp/fixture" "$temp/home"
export FLOW_FIXTURE_DIR="$temp/fixture"
printf '#!/bin/sh\n[ "${1:-}" = --version ] && printf "gasa fixture 1.2.3\\n"\n' > "$temp/fixture/gasa"
while [ "$(wc -c < "$temp/fixture/gasa" | tr -d '[:space:]')" -lt 1024 ]; do printf '# fixture padding for installer size check\n' >> "$temp/fixture/gasa"; done
chmod +x "$temp/fixture/gasa"
tar -czf "$FLOW_FIXTURE_DIR/archive.tar.gz" -C "$FLOW_FIXTURE_DIR" gasa
archive_hash=$(sha256sum "$FLOW_FIXTURE_DIR/archive.tar.gz" | awk '{print $1}')
printf '%s  gasa_1.2.3_linux_amd64.tar.gz\n' "$archive_hash" > "$FLOW_FIXTURE_DIR/checksums.txt"
printf '%064d  gasa_1.2.3_linux_amd64.tar.gz\n' 0 > "$FLOW_FIXTURE_DIR/checksums-bad.txt"
printf '%s  gasa_1.2.3_linux_amd64.tar.gz\n%s  gasa_1.2.3_linux_amd64.tar.gz\n' "$archive_hash" "$(printf '%064d' 1)" > "$FLOW_FIXTURE_DIR/checksums-conflict.txt"
install_dir="$temp/installed/gasa-flow/bin"
set +e
PATH="$temp/bin:$PATH" HOME="$temp/home" TMPDIR="$temp" GASA_VERSION=v1.2.3 GASA_INSTALL_DIR="$install_dir" sh "$installer" > "$temp/install.out" 2>&1
install_status=$?
set -e
if [ "$install_status" -eq 0 ] && [ -x "$install_dir/gasa" ] && "$install_dir/gasa" --version >/dev/null; then printf 'PASS: matching checksum installs archive in temporary destination\n'; else printf 'FAIL: matching checksum did not install (exit=%s)\n' "$install_status"; cat "$temp/install.out"; failed=1; fi
bad_dir="$temp/failed/gasa-flow/bin"
set +e
PATH="$temp/bin:$PATH" HOME="$temp/home" TMPDIR="$temp" FLOW_BAD_CHECKSUM=1 GASA_VERSION=v1.2.3 GASA_INSTALL_DIR="$bad_dir" sh "$installer" > "$temp/bad-install.out" 2>&1
bad_status=$?
set -e
if [ "$bad_status" -ne 0 ] && [ ! -e "$bad_dir/gasa" ]; then printf 'PASS: failed checksum leaves destination uninstalled\n'; else printf 'FAIL: failed checksum installed a binary\n'; failed=1; fi
conflict_dir="$temp/conflict/gasa-flow/bin"
set +e
PATH="$temp/bin:$PATH" HOME="$temp/home" TMPDIR="$temp" FLOW_CONFLICT_CHECKSUM=1 GASA_VERSION=v1.2.3 GASA_INSTALL_DIR="$conflict_dir" sh "$installer" > "$temp/conflict.out" 2>&1
conflict_status=$?
set -e
if [ "$conflict_status" -ne 0 ] && [ ! -e "$conflict_dir/gasa" ] && grep -Fq 'conflicting checksums' "$temp/conflict.out"; then printf 'PASS: conflicting duplicate archive hashes abort installation\n'; else printf 'FAIL: conflicting duplicate hashes were not rejected\n'; failed=1; fi
dd if=/dev/zero of="$FLOW_FIXTURE_DIR/gasa" bs=1048576 count=100 2>/dev/null
printf x >> "$FLOW_FIXTURE_DIR/gasa"
tar -czf "$FLOW_FIXTURE_DIR/archive-large.tar.gz" -C "$FLOW_FIXTURE_DIR" gasa
large_hash=$(sha256sum "$FLOW_FIXTURE_DIR/archive-large.tar.gz" | awk '{print $1}')
printf '%s  gasa_1.2.3_linux_amd64.tar.gz\n' "$large_hash" > "$FLOW_FIXTURE_DIR/checksums-large.txt"
large_dir="$temp/large/gasa-flow/bin"
set +e
PATH="$temp/bin:$PATH" HOME="$temp/home" TMPDIR="$temp" FLOW_OVERSIZE=1 GASA_VERSION=v1.2.3 GASA_INSTALL_DIR="$large_dir" sh "$installer" > "$temp/large.out" 2>&1
large_status=$?
set -e
if [ "$large_status" -ne 0 ] && [ ! -e "$large_dir/gasa" ] && grep -Fq '100 MiB' "$temp/large.out"; then printf 'PASS: extracted executable over 100 MiB is rejected\n'; else printf 'FAIL: oversized extracted executable was not rejected\n'; failed=1; fi
if [ -z "$(find "$temp" -maxdepth 1 -type d -name 'gasacli.*' -print | head -n 1)" ]; then printf 'PASS: installer temporary directory is cleaned after completion and checksum failure\n'; else printf 'FAIL: installer temporary directory leaked\n'; failed=1; fi
hang_marker="$temp/curl-started"
PATH="$temp/bin:$PATH" HOME="$temp/home" TMPDIR="$temp" FLOW_HANG=1 FLOW_HANG_MARKER="$hang_marker" GASA_VERSION=v1.2.3 GASA_INSTALL_DIR="$temp/interrupted/gasa-flow/bin" sh "$installer" > "$temp/interrupted.out" 2>&1 &
hang_pid=$!
attempt=0
while [ ! -f "$hang_marker" ] && [ "$attempt" -lt 100 ]; do sleep 0.05; attempt=$((attempt + 1)); done
if [ -f "$hang_marker" ]; then
  kill -TERM "$hang_pid" 2>/dev/null || :
  set +e; wait "$hang_pid"; interrupted_status=$?; set -e
  if [ "$interrupted_status" -ne 0 ] && [ -z "$(find "$temp" -maxdepth 1 -type d -name 'gasacli.*' -print | head -n 1)" ]; then printf 'PASS: SIGTERM interruption removes shell temporary directory\n'; else printf 'FAIL: SIGTERM interruption cleanup failed\n'; failed=1; fi
else
  printf 'FAIL: interruption fixture did not reach fake curl\n'; kill -TERM "$hang_pid" 2>/dev/null || :; failed=1
fi
exit "$failed"
