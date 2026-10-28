#!/bin/sh
set -eu
installer=$(CDPATH= cd -- "$(dirname "$0")/../../scripts" && pwd -P)/install.sh
temp=$(mktemp -d "${TMPDIR:-/tmp}/gasa-policy.XXXXXX")
temp=$(CDPATH= cd -P -- "$temp" && pwd -P)
fakebin="$temp/fakebin"
mkdir -p "$fakebin"
cat > "$fakebin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in -m) printf '%s\n' x86_64 ;; *) printf '%s\n' Linux ;; esac
EOF
chmod +x "$fakebin/uname"
PATH="$fakebin:$PATH"
export PATH
failed=0
check() {
    name=$1 expected=$2 path=$3
    if [ "$expected" = accept ]; then wanted=0; else wanted=3; fi
    set +e
    output=$(sh "$installer" --test-install-dir "$path" 2>&1)
    status=$?
    set -e
    if [ "$status" -eq "$wanted" ]; then printf 'PASS: %s\n' "$name"
    else printf 'FAIL: %s expected=%s got=%s output=%s\n' "$name" "$wanted" "$status" "$output"; failed=1; fi
}
check_url() {
    name=$1 expected=$2 url=$3
    if [ "$expected" = allow ]; then wanted=0; else wanted=3; fi
    set +e
    output=$(sh "$installer" --test-effective-url "$url" 2>&1)
    status=$?
    set -e
    if [ "$status" -eq "$wanted" ]; then printf 'PASS: %s\n' "$name"
    else printf 'FAIL: %s expected=%s got=%s output=%s\n' "$name" "$wanted" "$status" "$output"; failed=1; fi
}

check 'default' accept "$HOME/.local/bin"
check 'temp gasacli' accept "$temp/gasacli"
check 'space in path' accept "$temp/gasa space"
check 'custom gasa prefix case insensitive' accept "$temp/GaSa-case"
check 'temp gasa-test bin' accept "$temp/gasa-test/bin"
check 'bin leaf case insensitive' accept "$temp/gasa-test/BiN"
check 'home local bin' accept "$HOME/.local/bin"
check 'home tools gasa' accept "$HOME/tools/gasa"
check 'home trailing slash' reject "$HOME/"
check 'home dot' reject "$HOME/."
check 'home parent' reject "$HOME/../$(basename "$HOME")"
check 'dot' reject .
check 'relative path' reject relative/dir
check 'backslash rejected on Unix' reject "$temp/gasa\\bin"
check 'semicolon rejected' reject "$temp/gasa;evil/bin"
check 'percent rejected' reject "$temp/gasa%VAR%/bin"
check 'asterisk rejected' reject "$temp/gasa*/bin"
check 'question rejected' reject "$temp/gasa?/bin"
check 'quote rejected' reject "$temp/gasa\"x/bin"
check 'less-than rejected' reject "$temp/gasa<x/bin"
check 'greater-than rejected' reject "$temp/gasa>x/bin"
check 'pipe rejected' reject "$temp/gasa|x/bin"
check 'dollar rejected' reject "$temp/gasa\$x/bin"
check 'backtick rejected' reject "$temp/gasa\`x/bin"
check 'colon rejected' reject "$temp/gasa:x/bin"
check 'control rejected' reject "$(printf "$temp/gasa\tx/bin")"
check 'newline rejected' reject "$(printf '%s/a\nb' "$temp")"
check 'carriage return rejected' reject "$(printf '%s/a\rb' "$temp")"
check 'tab rejected' reject "$(printf '%s/a\tb' "$temp")"
check 'DEL rejected' reject "$(printf '%s/a\177b' "$temp")"
check 'C1 U+0080 rejected' reject "$(printf '%s/a\302\200b' "$temp")"
check 'C1 U+009F rejected' reject "$(printf '%s/a\302\237b' "$temp")"
check 'direction mark U+200E rejected' reject "$(printf '%s/a\342\200\216b' "$temp")"
check 'line separator U+2028 rejected' reject "$(printf '%s/a\342\200\250b' "$temp")"
check 'bidi override U+202E rejected' reject "$(printf '%s/a\342\200\256b' "$temp")"
check 'bidi isolate U+2066 rejected' reject "$(printf '%s/a\342\201\246b' "$temp")"
check 'trailing newline rejected' reject "$(printf '%s/a\n' "$temp")"
check 'root' reject /
check '/usr' reject /./usr
check '/usr descendant' reject /usr/local/gasa
check '/tmp parent traversal' reject /tmp/../usr
check '/usr/bin' reject /usr/bin
check '/bin' reject /bin
check '/sbin' reject /sbin
check '/etc' reject /etc
check '/var' reject /var
check '/opt' reject /opt
check '/lib*' reject /lib64
check '/tmp root' reject /tmp
check '/Users' reject /Users
check '/Users/Shared descendant' reject /Users/Shared/gasa
check '/home' reject /home
check '/System' reject /System
check '/System descendant' reject /System/gasa
check '/Library' reject /Library
check '/Library descendant' reject /Library/gasa
check '/Applications' reject /Applications
check '/Applications descendant' reject /Applications/gasa
check '/c/Users' reject /c/Users/
check_url 'HTTPS release host without port allowed' allow 'https://github.com/releases/file'
check_url 'HTTPS explicit port 443 allowed' allow 'https://github.com:443/releases/file'
check_url 'HTTPS port 8443 rejected' reject 'https://github.com:8443/releases/file'
check_url 'HTTP final URL rejected' reject 'http://github.com/releases/file'
check_url 'uppercase GitHub hostname allowed' allow 'https://GitHub.com/releases/file'
checksum_file="$temp/checksums.txt"
checksum_name='gasa_1.2.3_linux_amd64.tar.gz'
hash_a='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
hash_b='bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
printf '%s  %s\r\n%s  %s\r\n' "$hash_a" "$checksum_name" "$hash_a" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = "$hash_a" ] && printf 'PASS: CRLF and identical duplicate checksum accepted\n' || { printf 'FAIL: CRLF checksum parse returned %s\n' "$result"; failed=1; }
printf '%s  %s\n%s  %s\n' "$hash_a" "$checksum_name" "$hash_b" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = CONFLICT ] && printf 'PASS: conflicting checksums rejected\n' || { printf 'FAIL: conflicting checksum parse returned %s\n' "$result"; failed=1; }
printf '%s  %s\n%s  %s\n' "$hash_b" "$checksum_name" "$hash_a" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = CONFLICT ] && printf 'PASS: reverse-order conflicting checksums rejected\n' || { printf 'FAIL: reverse-order checksum parse returned %s\n' "$result"; failed=1; }
printf '%s *%s\n%s  %s.sig\n%s  %s.sbom\n' "$hash_a" "$checksum_name" "$hash_b" "$checksum_name" "$hash_b" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = "$hash_a" ] && printf 'PASS: binary checksum matches; suffixed names are ignored\n' || { printf 'FAIL: binary/suffix checksum parse returned %s\n' "$result"; failed=1; }
printf '\357\273\277 \t%s\t*%s \r\n%s %s.sig\n%s %s.sbom\n' "$hash_a" "$checksum_name" "$hash_b" "$checksum_name" "$hash_b" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = "$hash_a" ] && printf 'PASS: BOM, leading whitespace, tab, CRLF, and exact archive name accepted\n' || { printf 'FAIL: normalized checksum parse returned %s\n' "$result"; failed=1; }
printf '%s  %s\n' "$(printf '%s' "$hash_a" | tr '[:lower:]' '[:upper:]')" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = "$hash_a" ] && printf 'PASS: uppercase checksum normalized\n' || { printf 'FAIL: uppercase checksum parse returned %s\n' "$result"; failed=1; }
printf '%s\302\240%s\n' "$hash_a" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = MISSING ] && printf 'PASS: NBSP is not a checksum separator\n' || { printf 'FAIL: NBSP checksum parse returned %s\n' "$result"; failed=1; }
printf '%s\342\200\200%s\n' "$hash_a" "$checksum_name" > "$checksum_file"
result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
[ "$result" = MISSING ] && printf 'PASS: U+3000 is not a checksum separator\n' || { printf 'FAIL: U+3000 checksum parse returned %s\n' "$result"; failed=1; }
while IFS='|' read -r case_name expected encoded; do
    [ -n "$case_name" ] || continue
    node -e 'process.stdout.write(Buffer.from(process.argv[1],"base64"))' "$encoded" > "$checksum_file"
    result=$(sh "$installer" --test-checksum "$checksum_file" "$checksum_name")
    if [ "$result" = "$expected" ]; then printf 'PASS: shared checksum fixture %s\n' "$case_name"
    else printf 'FAIL: shared checksum fixture %s expected=%s got=%s\n' "$case_name" "$expected" "$result"; failed=1; fi
done < "$(dirname "$0")/fixtures/checksum-parity.txt"
recovery="$temp/recovery"
mkdir -p "$recovery"
printf partial > "$recovery/.gasa.orphan.tmp"
printf older > "$recovery/gasa.old.older"
printf newest > "$recovery/gasa.old.newest"
touch -t 202001010000 "$recovery/gasa.old.older"
touch -t 202101010000 "$recovery/gasa.old.newest"
sh "$installer" --test-recover "$recovery" || failed=1
if [ "$(cat "$recovery/gasa" 2>/dev/null)" = newest ] && [ ! -e "$recovery/.gasa.orphan.tmp" ]; then printf 'PASS: interrupted install restores the newest backup by mtime\n'; else printf 'FAIL: interrupted install recovery failed\n'; failed=1; fi
lock_test="$temp/lock-test"
mkdir -p "$lock_test/.gasa-install.lock"
printf '%s %s\n' 99999999 "$(date +%s)" > "$lock_test/.gasa-install.lock/owner"
sh "$installer" --test-lock "$lock_test" && [ ! -e "$lock_test/.gasa-install.lock" ] && printf 'PASS: orphan install lock is discarded\n' || { printf 'FAIL: orphan install lock was not discarded\n'; failed=1; }
mkdir -p "$lock_test/.gasa-install.lock"
set +e
fresh_lock_output=$(sh "$installer" --test-lock "$lock_test" 2>&1); fresh_lock_status=$?
set -e
if [ "$fresh_lock_status" -ne 0 ] && [ -d "$lock_test/.gasa-install.lock" ] && [ ! -e "$lock_test/.gasa-install.lock/owner" ]; then printf 'PASS: ownerless lock younger than 60 seconds is preserved\n'; else printf 'FAIL: fresh ownerless lock was reassigned\n'; failed=1; fi
touch -t 202001010000 "$lock_test/.gasa-install.lock"
sh "$installer" --test-lock "$lock_test" && [ ! -e "$lock_test/.gasa-install.lock" ] && printf 'PASS: ownerless lock older than 60 seconds is recoverable\n' || { printf 'FAIL: stale ownerless lock was not recovered\n'; failed=1; }
mkdir -p "$lock_test/.gasa-install.lock"
printf '%s %s\n' "$$" "$(date +%s)" > "$lock_test/.gasa-install.lock/owner"
set +e
lock_output=$(sh "$installer" --test-lock "$lock_test" 2>&1); lock_status=$?
set -e
if [ "$lock_status" -ne 0 ] && [ -d "$lock_test/.gasa-install.lock" ]; then printf 'PASS: live install lock refuses second installer\n'; else printf 'FAIL: live install lock was not respected\n'; failed=1; fi
set +e
third_lock_output=$(sh "$installer" --test-lock "$lock_test" 2>&1); third_lock_status=$?
set -e
if [ "$third_lock_status" -ne 0 ] && [ -d "$lock_test/.gasa-install.lock" ] && [ "$(awk '{print $1}' "$lock_test/.gasa-install.lock/owner")" = "$$" ]; then printf 'PASS: third installer is refused and live owner lock survives\n'; else printf 'FAIL: third installer altered live owner lock\n'; failed=1; fi
mkdir_path="$temp/created/gasa-bin"
sh "$installer" --test-mkdir "$mkdir_path" || failed=1
mode=$(stat -c '%a' "$temp/created" 2>/dev/null || stat -f '%Lp' "$temp/created")
[ "$mode" = 755 ] && printf 'PASS: install directory created with umask 022\n' || { printf 'FAIL: install directory mode is %s\n' "$mode"; failed=1; }
backup_a=$(sh "$installer" --test-backup "$temp/gasa")
backup_b=$(sh "$installer" --test-backup "$temp/gasa")
[ "$backup_a" != "$backup_b" ] && printf 'PASS: update backup names are unique\n' || { printf 'FAIL: update backup name collided\n'; failed=1; }

link="$temp/gasa-link"
if [ "${OS:-}" = Windows_NT ]; then
    link_win=$(cygpath -w "$link"); home_win=$(cygpath -w "$HOME")
    if MSYS_NO_PATHCONV=1 cmd.exe /d /c mklink /J "$link_win" "$home_win" >/dev/null 2>&1; then
        check 'temporary reparse point to HOME' reject "$link"
        MSYS_NO_PATHCONV=1 cmd.exe /d /c rmdir "$link_win" >/dev/null 2>&1 || failed=1
    else printf 'FAIL: could not create test junction\n'; failed=1; fi
else
    if ln -s "$HOME" "$link" 2>/dev/null; then check 'temporary symlink to HOME' reject "$link"; else printf 'FAIL: could not create symlink test\n'; failed=1; fi
    rm -f "$link"
fi
for flavor in MINGW_NT MSYS_NT CYGWIN_NT; do
    cat > "$fakebin/uname" <<EOF
#!/bin/sh
printf '%s\n' $flavor-10.0
EOF
    chmod +x "$fakebin/uname"
    set +e
    output=$(PATH="$fakebin:$PATH" sh "$installer" 2>&1)
    status=$?
    set -e
    if [ "$status" -eq 0 ] || ! printf '%s' "$output" | grep -Fq 'Use scripts/install.ps1 no Windows'; then printf 'FAIL: %s refusal expected exit 1 and message; got exit=%s output=%s\n' "$flavor" "$status" "$output"; failed=1
    else printf 'PASS: %s is refused with the Windows installer message\n' "$flavor"; fi
done
rm -rf "$temp"
exit "$failed"
