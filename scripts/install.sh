#!/bin/sh
set -eu
umask 077

case "$(uname -s 2>/dev/null || true)" in MINGW*|MSYS*|CYGWIN*) printf '%s\n' 'Use scripts/install.ps1 no Windows' >&2; exit 1 ;; esac

repo='GasaTec/gasacli'
release_base="https://github.com/$repo/releases/download"
install_dir=${GASA_INSTALL_DIR:-"$HOME/.local/bin"}
newline='
'
validate_effective_url() {
    effective=$1
    case "$effective" in https://*) authority=${effective#https://} ;; *) return 1 ;; esac
    authority=${authority%%/*}
    case "$authority" in *@*|*\[*|*\]*) return 1 ;; esac
    case "$authority" in *:*) host=${authority%%:*}; port=${authority#*:}; [ "$port" = 443 ] || return 1 ;; *) host=$authority ;; esac
    host=$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')
    case "$host" in github.com|api.github.com|objects.githubusercontent.com|release-assets.githubusercontent.com) return 0 ;; *) return 1 ;; esac
}
canonical_dir() {
    input=$1
    case "$input" in *\\*) return 1 ;; esac
    case "$input" in /*) ;; *) return 1 ;; esac
    rest=$input; suffix=
    while [ ! -d "$rest" ]; do
        [ "$rest" != / ] || return 1
        base=${rest##*/}; parent=${rest%/*}; [ -n "$parent" ] || parent=/
        suffix=/$base$suffix; rest=$parent
    done
    check=$rest
    while [ "$check" != / ]; do
        [ ! -L "$check" ] || return 2
        if [ "${OS:-}" = Windows_NT ] && command -v fsutil.exe >/dev/null 2>&1; then
            win_check=$(cygpath -w "$check" 2>/dev/null) || return 1
            MSYS_NO_PATHCONV=1 fsutil.exe reparsepoint query "$win_check" >/dev/null 2>&1 && return 2
        fi
        check=${check%/*}; [ -n "$check" ] || check=/
    done
    [ ! -L / ] || return 2
    physical=$(CDPATH='' cd -P "$rest" 2>/dev/null && pwd -P) || return 1
    canonical_result="${physical%/}$suffix"
    [ -n "$canonical_result" ] || canonical_result=/
    printf '%s\n' "$canonical_result"
}
safe_install_dir() {
    unsafe_reason=''
    case "$1" in *[[:cntrl:]]*|*"$newline"*) unsafe_reason='install directory contains a prohibited character'; return 1 ;; esac
    case "$1" in *';'*|*'%'*|*'*'*|*'?'*|*'"'*|*'<'*|*'>'*|*'|'*|*'$'*|*'`'*|*\\*|*':'*) unsafe_reason='install directory contains a prohibited character'; return 1 ;; esac
    if printf '%s' "$1" | LC_ALL=C grep -q '[[:cntrl:]]'; then unsafe_reason='install directory contains a prohibited character'; return 1; fi
    if printf '%s' "$1" | LC_ALL=C grep -Eq '(\302[\200-\237]|\342\200[\216\217\250-\256]|\342\201[\246-\251])'; then unsafe_reason='install directory contains a prohibited Unicode control'; return 1; fi
    candidate=$(canonical_dir "$1") || { unsafe_reason='path must be absolute and contain no symlink or reparse point'; return 1; }
    standard=$(canonical_dir "$HOME/.local/bin") || standard=
    [ "$candidate" = "$standard" ] && return 0
    leaf=$(printf '%s' "${candidate##*/}" | tr '[:upper:]' '[:lower:]')
    lower_candidate=$(printf '%s' "$candidate" | tr '[:upper:]' '[:lower:]')
    case "$leaf" in bin|gasa*) ;; *) unsafe_reason='custom directory must end in bin or a name beginning with gasa'; return 1 ;; esac
    case "$lower_candidate" in
        /|/usr|/usr/*|/bin|/bin/*|/sbin|/sbin/*|/etc|/etc/*|/var|/var/*|/opt|/opt/*|/lib*) unsafe_reason='directory is a protected system location'; return 1 ;;
    esac
    for protected in /usr /bin /sbin /etc /var /opt /lib /Users/Shared /System /Library /Applications; do
        protected_lc=$(printf '%s' "$protected" | tr '[:upper:]' '[:lower:]')
        case "$protected_lc/" in "$lower_candidate/"*) unsafe_reason='directory is a protected location or its ancestor'; return 1 ;; esac
        case "$lower_candidate/" in "$protected_lc/"*) unsafe_reason='directory is a protected location'; return 1 ;; esac
    done
    return 0
}
mkdir_install_dir() { (umask 022; mkdir -p "$1"); }
backup_path() { printf '%s.old.%s.%s\n' "$1" "$(date -u +%Y%m%d%H%M%S)" "$$"; }
write_install_lock_owner() {
    lock_owner_record="$$ $(date +%s)"
    owner_tmp="$lock/.owner.$$"
    if printf '%s\n' "$lock_owner_record" > "$owner_tmp" && mv -f "$owner_tmp" "$lock/owner"; then return 0; fi
    rm -f "$owner_tmp"; rm -rf "$lock"; lock_owned=0; return 1
}
release_install_lock() {
    [ "${lock_owned:-0}" -eq 1 ] || return 0
    if [ -f "$lock/owner" ] && IFS= read -r current_owner < "$lock/owner" && [ "$current_owner" = "$lock_owner_record" ]; then rm -rf "$lock"; fi
    lock_owned=0
}
recover_install_state() {
    dir=$1
    find "$dir" -maxdepth 1 -type f -name '.gasa.*' -delete 2>/dev/null || :
    target="$dir/gasa"
    if [ ! -e "$target" ]; then
        # shellcheck disable=SC2012 # mtime ordering is the point; names are created by this installer
        backup=$(ls -t "$dir"/gasa.old* 2>/dev/null | head -n 1 || :)
        [ -z "$backup" ] || mv "$backup" "$target" || return 1
    fi
}
acquire_install_lock() {
    lock="$1/.gasa-install.lock"
    if mkdir "$lock" 2>/dev/null; then
        lock_owned=1
        write_install_lock_owner || return 1
        return 0
    fi
    lock_mtime=$(stat -c %Y "$lock" 2>/dev/null || stat -f %m "$lock" 2>/dev/null || printf '0')
    case "$lock_mtime" in ''|*[!0-9]*) lock_age=0 ;; *) lock_age=$(($(date +%s) - lock_mtime)) ;; esac
    if [ ! -f "$lock/owner" ]; then
        [ "$lock_age" -gt 60 ] || return 1
        rm -rf "$lock" 2>/dev/null || return 1
        mkdir "$lock" 2>/dev/null || return 1
        lock_owned=1
        write_install_lock_owner || return 1
        return 0
    fi
    owner_pid=''; owner_time=''
    if [ -f "$lock/owner" ]; then read -r owner_pid owner_time < "$lock/owner" || :; fi
    now=$(date +%s); age=999999
    case "$owner_time" in ''|*[!0-9]*) ;; *) age=$((now - owner_time)) ;; esac
    if [ "$age" -gt 1800 ] || { case "$owner_pid" in ''|*[!0-9]*) true ;; *) ! kill -0 "$owner_pid" 2>/dev/null ;; esac; }; then
        rm -rf "$lock" 2>/dev/null || return 1
        mkdir "$lock" 2>/dev/null || return 1
        lock_owned=1
        write_install_lock_owner || return 1
        return 0
    fi
    return 1
}
msg() { printf '%s / %s\n' "$1" "$2"; }
fail() { msg "Erro: $1" "Error: $2" >&2; exit 1; }
checksum_for_archive() {
    LC_ALL=C tr '\000' '\001' < "$1" | LC_ALL=C awk -v name="$2" 'NR == 1 { sub(/^\357\273\277/, "") } { sub(/\r$/, ""); line=$0; sub(/^[ \t\v\f]*/, "", line); hash=line; sub(/[ \t\v\f].*$/, "", hash); rest=substr(line,length(hash)+1); if (length(hash) == 64 && hash ~ /^[[:xdigit:]]+$/ && rest ~ /^[ \t\v\f]+/) { sub(/^[ \t\v\f]+/, "", rest); sub(/^\*/, "", rest); sub(/[ \t\v\f]*$/, "", rest); if (rest !~ /[ \t\v\f\r]/ && rest == name) { h=tolower(hash); if (found && h != expected) conflict=1; expected=h; found=1 } } } END { if (conflict) print "CONFLICT"; else if (found) print expected; else print "MISSING" }'
}
download() {
    current=$1; dest=$2; hop=0
    while [ "$hop" -le 5 ]; do
        validate_effective_url "$current" || { rm -f "$dest"; return 1; }
        headers="$dest.headers"; body="$dest.part"; rm -f "$headers" "$body"
        status=$(curl -fsS --proto '=https' --max-time 120 --max-filesize 209715200 -H 'User-Agent: gasacli-installer' -D "$headers" -o "$body" -w '%{http_code}' "$current") || { rm -f "$dest" "$headers" "$body"; return 1; }
        case "$status" in 301|302|303|307|308)
            [ "$hop" -lt 5 ] || { rm -f "$dest" "$headers" "$body"; return 1; }
            location=$(awk 'BEGIN{FS=":"} { sub(/\r$/, ""); if (tolower($1) == "location") { sub(/^[^:]*:[[:space:]]*/, ""); value=$0 } } END{print value}' "$headers")
            [ -n "$location" ] || { rm -f "$dest" "$headers" "$body"; return 1; }
            location_scheme=$(printf '%s' "$location" | cut -d: -f1 | tr '[:upper:]' '[:lower:]')
            location_tail=${location#*:}
            case "$location_scheme:$location_tail" in
                http://*) rm -f "$dest" "$headers" "$body"; return 1 ;;
                https://*) current="https:$location_tail" ;;
                /*) authority=${current#https://}; authority=${authority%%/*}; current="https://$authority$location" ;;
                \?*) authority=${current#https://}; authority=${authority%%/*}; current_path=${current#"https://$authority"}; current_path=${current_path%%\?*}; current="https://$authority$current_path$location" ;;
                *) authority=${current#https://}; authority=${authority%%/*}; current_path=${current#"https://$authority"}; current_path=${current_path%%\?*}; current_dir=${current_path%/*}; [ -n "$current_dir" ] || current_dir=/; current="https://$authority$current_dir/$location" ;;
            esac
            validate_effective_url "$current" || { rm -f "$dest" "$headers" "$body"; return 1; }
            rm -f "$headers" "$body"; hop=$((hop + 1)) ;;
        *) mv -f "$body" "$dest" || { printf '%s\n' "Could not move downloaded file into place: $dest" >&2; return 1; }; rm -f "$headers"; return 0 ;;
        esac
    done
    return 1
}
get_latest_version() {
    response_file=$(mktemp "${TMPDIR:-/tmp}/gasacli-latest.XXXXXX") || return 1
    if download "https://api.github.com/repos/$repo/releases?per_page=1" "$response_file"; then cat "$response_file"; status=0; else status=1; fi
    rm -f "$response_file"
    return "$status"
}
if [ "${1:-}" = '--test-install-dir' ]; then
    safe_install_dir "${2:-}" && { printf 'SAFE\n'; exit 0; }
    printf 'REJECTED\n'; exit 3
fi
if [ "${1:-}" = '--test-effective-url' ]; then
    validate_effective_url "${2:-}" && { printf 'ALLOWED\n'; exit 0; }
    printf 'REJECTED\n'; exit 3
fi
if [ "${1:-}" = '--test-checksum' ]; then
    checksum_for_archive "${2:-}" "${3:-}"; exit 0
fi
if [ "${1:-}" = '--test-download' ]; then download "${2:-}" "${3:-}"; exit $?; fi
if [ "${1:-}" = '--test-recover' ]; then
    recover_install_state "${2:-}" && exit 0; exit 1
fi
if [ "${1:-}" = '--test-lock' ]; then mkdir -p "${2:-}"; lock_owned=0; acquire_install_lock "${2:-}" && { release_install_lock; exit 0; }; exit 1; fi
if [ "${1:-}" = '--test-mkdir' ]; then
    mkdir_install_dir "${2:-}" && exit 0; exit 1
fi
if [ "${1:-}" = '--test-backup' ]; then
    backup_path "${2:-}"; exit 0
fi

if [ "${GASA_UNINSTALL:-}" = 1 ]; then
    safe_install_dir "$install_dir" || fail "Install directory rejected: $unsafe_reason ($install_dir)" "Install directory rejected: $unsafe_reason ($install_dir)"
    [ "$install_dir" = "$HOME/.local/bin" ] || install_dir=$(canonical_dir "$install_dir") || fail "Invalid install directory." "Invalid install directory."
    lock_owned=0
    acquire_install_lock "$install_dir" || fail "Outra instalação está em andamento." "another installation is in progress"
    trap release_install_lock EXIT
    if [ ! -f "$install_dir/gasa" ] || [ -L "$install_dir/gasa" ]; then fail "gasa não encontrado como arquivo regular em $install_dir." "gasa was not found as a regular file in $install_dir."; fi
    rm -f "$install_dir/gasa"
    find "$install_dir" -maxdepth 1 -type f -name 'gasa.old*' -mtime +0 -exec rm -f {} \; 2>/dev/null || :
    rmdir "$install_dir" 2>/dev/null || :
    msg "Binário removido; os dados em ~/.config/gasacli/ foram preservados." "Binary removed; data in ~/.config/gasacli/ was preserved."
    exit 0
fi

if [ "${GASA_METHOD:-binary}" = npm ]; then
    command -v npm >/dev/null 2>&1 || fail "npm não encontrado; use o instalador binário direto." "npm was not found; use the direct binary installer."
    version=${GASA_VERSION:-}
    if [ -z "$version" ]; then
        latest=$(get_latest_version) || fail "Não foi possível consultar as releases do GitHub." "Could not query GitHub releases."
        version=$(printf '%s\n' "$latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
    fi
    case "$version" in *"$newline"*|*"$(printf '\r')"*) fail "Versão inválida." "Invalid version." ;; esac
    printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$' || fail "Versão inválida: $version" "Invalid version: $version"
    npm install -g "gasacli@${version#v}" || fail "npm install -g gasacli@${version#v} falhou." "npm install -g gasacli@${version#v} failed."
    command -v gasa >/dev/null 2>&1 || fail "Instalado pelo npm, mas gasa não está no PATH." "Installed with npm, but gasa is not on PATH."
    gasa --version || fail "gasa --version falhou após a instalação." "gasa --version failed after installation."
    exit 0
fi
[ "${GASA_METHOD:-binary}" = binary ] || fail "GASA_METHOD deve ser binary ou npm." "GASA_METHOD must be binary or npm."
safe_install_dir "$install_dir" || fail "Install directory rejected: $unsafe_reason ($install_dir)" "Install directory rejected: $unsafe_reason ($install_dir)"
[ "$install_dir" = "$HOME/.local/bin" ] || install_dir=$(canonical_dir "$install_dir") || fail "Invalid install directory." "Invalid install directory."
command -v curl >/dev/null 2>&1 || fail "curl é necessário para downloads seguros." "curl is required for secure downloads."

os=$(uname -s)
machine=$(uname -m)
case "$os" in Linux) platform=linux ;; Darwin) platform=darwin ;; *) fail "Sistema não suportado: $os" "Unsupported operating system: $os" ;; esac
case "$machine" in x86_64|amd64|AMD64) arch=amd64 ;; aarch64|arm64|ARM64) arch=arm64 ;; *) fail "Arquitetura não suportada: $machine" "Unsupported architecture: $machine" ;; esac

version=${GASA_VERSION:-}
if [ -z "$version" ]; then
latest=$(get_latest_version) || fail "Não foi possível consultar as releases do GitHub." "Could not query GitHub releases."
    version=$(printf '%s\n' "$latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
fi
case "$version" in *"$newline"*|*"$(printf '\r')"*) fail "Versão inválida." "Invalid version." ;; esac
printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$' || fail "Versão inválida: $version" "Invalid version: $version"
archive="gasa_${version#v}_${platform}_${arch}.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then hash_tool='shasum -a 256'
else fail "sha256sum ou shasum é necessário para verificar o download." "sha256sum or shasum is required to verify the download."; fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/gasacli.XXXXXX") || fail "Não foi possível criar diretório temporário." "Could not create temporary directory."
lock=
lock_owned=0
staged=
cleanup() { [ -z "$staged" ] || rm -f "$staged"; [ -z "$tmp" ] || rm -rf "$tmp"; release_install_lock; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP
download "$release_base/$version/checksums.txt" "$tmp/checksums.txt" || fail "Falha ao baixar checksums.txt de um host permitido." "Could not download checksums.txt from an allowed host."
download "$release_base/$version/$archive" "$tmp/$archive" || fail "Falha ao baixar $archive de um host permitido." "Could not download $archive from an allowed host."
checksum_result=$(checksum_for_archive "$tmp/checksums.txt" "$archive")
[ "$checksum_result" != CONFLICT ] || fail "Checksums conflitantes para $archive." "conflicting checksums for $archive"
expected=$checksum_result
 [ "$expected" != MISSING ] || fail "Archive ausente em checksums.txt." "Archive missing from checksums.txt."
case "$expected" in ''|*[!0-9a-f]*) fail "Archive ausente ou checksum inválido em checksums.txt." "Archive missing or invalid checksum in checksums.txt." ;; esac
[ "${#expected}" -eq 64 ] || fail "Checksum inválido para $archive." "Invalid checksum for $archive."
if [ "$hash_tool" = sha256sum ]; then actual=$(sha256sum "$tmp/$archive" | awk '{print tolower($1)}')
else actual=$(shasum -a 256 "$tmp/$archive" | awk '{print tolower($1)}'); fi
[ "$actual" = "$expected" ] || fail "SHA-256 divergente para $archive; nada foi instalado." "SHA-256 mismatch for $archive; nothing was installed."
mkdir -p "$tmp/extract" || fail "Não foi possível preparar extração." "Could not prepare extraction."
tar -tf "$tmp/$archive" > "$tmp/archive.names" || fail "Não foi possível validar os nomes do archive." "Could not inspect archive names."
tar -tvf "$tmp/$archive" > "$tmp/archive.types" || fail "Não foi possível validar os tipos do archive." "Could not inspect archive types."
[ "$(wc -l < "$tmp/archive.names" | tr -d '[:space:]')" -eq "$(wc -l < "$tmp/archive.types" | tr -d '[:space:]')" ] || fail "Nomes ambíguos no archive." "Ambiguous names in release archive."
gasa_count=0
set -f
exec 3< "$tmp/archive.types"
while IFS= read -r entry_name; do
    [ -n "$entry_name" ] || continue
    IFS= read -r entry <&3 || fail "Listagem de archive inconsistente." "Inconsistent archive listing."
    entry_type=${entry%"${entry#?}"}
    case "$entry_type:$entry_name" in
        -:gasa) gasa_count=$((gasa_count + 1)) ;;
        -:LICENSE|-:README.md) ;;
        *) fail "Entrada não permitida no archive: $entry_name" "Disallowed archive entry: $entry_name" ;;
    esac
done < "$tmp/archive.names"
exec 3<&-
set +f
[ "$gasa_count" -eq 1 ] || fail "Archive deve conter exatamente um arquivo regular gasa na raiz." "Archive must contain exactly one regular gasa file at its root."
tar -xf "$tmp/$archive" -C "$tmp/extract" || fail "Não foi possível extrair o archive." "Could not extract the archive."
if [ ! -f "$tmp/extract/gasa" ] || [ -L "$tmp/extract/gasa" ]; then fail "O archive não contém gasa como arquivo regular na raiz." "Archive does not contain gasa as a regular file at its root."; fi
[ -z "$(find "$tmp/extract" -type f -links +1 -print | head -n 1)" ] || fail "Hard links não são aceitos no archive." "Hard links are not accepted in the archive."
[ "$(wc -c < "$tmp/extract/gasa" | tr -d '[:space:]')" -le 104857600 ] || fail "Executável extraído excede 100 MiB." "Extracted executable exceeds 100 MiB."
[ "$(wc -c < "$tmp/extract/gasa")" -ge 1024 ] || fail "Executável extraído pequeno demais." "Extracted executable is too small."
chmod 755 "$tmp/extract/gasa"
"$tmp/extract/gasa" --version || fail "gasa --version falhou no binário novo; instalação não alterada." "New binary failed --version; installation was not changed."
mkdir_install_dir "$install_dir" || fail "Não foi possível criar diretório de instalação." "Could not create installation directory."
acquire_install_lock "$install_dir" || fail "Outra instalação está em andamento." "another installation is in progress"
recover_install_state "$install_dir" || fail "Não foi possível restaurar backup." "Could not restore interrupted-install backup."
staged=$(mktemp "$install_dir/.gasa.XXXXXX") || fail "Não foi possível preparar instalação." "Could not stage installation."
target="$install_dir/gasa"; backup=$(backup_path "$target")
cp "$tmp/extract/gasa" "$staged" || fail "Não foi possível preparar instalação." "Could not stage installation."
chmod 755 "$staged"
find "$install_dir" -maxdepth 1 -type f -name 'gasa.old*' -mtime +0 -exec rm -f {} \; 2>/dev/null || :
had_target=0
if [ -e "$target" ]; then mv -f "$target" "$backup" || fail "Não foi possível preservar binário anterior." "Could not preserve previous binary."; had_target=1; fi
if ! mv -f "$staged" "$target"; then
    [ "$had_target" -eq 0 ] || mv -f "$backup" "$target"
    fail "Não foi possível instalar gasa." "Could not install gasa."
fi
if ! "$target" --version; then
    rm -f "$target"
    [ "$had_target" -eq 0 ] || mv -f "$backup" "$target"
    fail "gasa --version falhou após instalação; backup restaurado." "gasa --version failed after installation; backup restored."
fi
rm -f "$backup"
case ":${PATH:-}:" in *:"$install_dir":*) : ;; *) msg "Adicione ao shell: export PATH=\"$install_dir:\$PATH\"" "Add to your shell: export PATH=\"$install_dir:\$PATH\"" ;; esac
msg "Instalado em $install_dir." "Installed to $install_dir."
