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
