#!/bin/sh
set -eu
umask 077

case "$(uname -s 2>/dev/null || true)" in MINGW*|MSYS*|CYGWIN*) printf '%s\n' 'Use scripts/install.ps1 no Windows' >&2; exit 1 ;; esac

repo='GasaTec/gasacli'
release_base="https://github.com/$repo/releases/download"
install_dir=${GASA_INSTALL_DIR:-"$HOME/.local/bin"}
validate_effective_url() {
    effective=$1
    case "$effective" in https://*) authority=${effective#https://} ;; *) return 1 ;; esac
    authority=${authority%%/*}
    case "$authority" in *@*|*\[*|*\]*) return 1 ;; esac
    case "$authority" in *:*) host=${authority%%:*}; port=${authority#*:}; [ "$port" = 443 ] || return 1 ;; *) host=$authority ;; esac
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
    physical=$(CDPATH= cd -P -- "$rest" 2>/dev/null && pwd -P) || return 1
    printf '%s\n' "${physical%/}$suffix" | sed 's#^$#/##'
}
safe_install_dir() {
    unsafe_reason=''
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
msg() { printf '%s / %s\n' "$1" "$2"; }
fail() { msg "Erro: $1" "Error: $2" >&2; exit 1; }
get_latest_version() {
    response=$(curl -fsSL --proto '=https' --proto-redir '=https' --max-redirs 5 --max-time 30 -H 'User-Agent: gasacli-installer' -w '\n%{url_effective}' "https://api.github.com/repos/$repo/releases?per_page=1") || return 1
    effective=$(printf '%s\n' "$response" | tail -n 1)
    validate_effective_url "$effective" || return 1
    printf '%s\n' "$response" | sed '$d'
}
newline='
'

if [ "${1:-}" = '--test-install-dir' ]; then
    safe_install_dir "${2:-}" && { printf 'SAFE\n'; exit 0; }
    printf 'REJECTED\n'; exit 3
fi
if [ "${1:-}" = '--test-effective-url' ]; then
    validate_effective_url "${2:-}" && { printf 'ALLOWED\n'; exit 0; }
    printf 'REJECTED\n'; exit 3
fi

if [ "${GASA_UNINSTALL:-}" = 1 ]; then
    safe_install_dir "$install_dir" || fail "Install directory rejected: $unsafe_reason ($install_dir)" "Install directory rejected: $unsafe_reason ($install_dir)"
    [ "$install_dir" = "$HOME/.local/bin" ] || install_dir=$(canonical_dir "$install_dir") || fail "Invalid install directory." "Invalid install directory."
    [ -f "$install_dir/gasa" ] && [ ! -L "$install_dir/gasa" ] || fail "gasa não encontrado como arquivo regular em $install_dir." "gasa was not found as a regular file in $install_dir."
    rm -f "$install_dir/gasa" "$install_dir/gasa.old"
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
    printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$' || fail "Versão inválida: $version" "Invalid version: $version"
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
printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$' || fail "Versão inválida: $version" "Invalid version: $version"
archive="gasa_${version#v}_${platform}_${arch}.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then hash_tool='shasum -a 256'
else fail "sha256sum ou shasum é necessário para verificar o download." "sha256sum or shasum is required to verify the download."; fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/gasacli.XXXXXX") || fail "Não foi possível criar diretório temporário." "Could not create temporary directory."
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
download() {
    url=$1; dest=$2
    effective=$(curl -fsSL --proto '=https' --proto-redir '=https' --max-redirs 5 --max-time 120 --max-filesize 209715200 -H 'User-Agent: gasacli-installer' -o "$dest" -w '%{url_effective}' "$url") || { rm -f "$dest"; return 1; }
    validate_effective_url "$effective" || { rm -f "$dest"; return 1; }
}
download "$release_base/$version/checksums.txt" "$tmp/checksums.txt" || fail "Falha ao baixar checksums.txt de um host permitido." "Could not download checksums.txt from an allowed host."
download "$release_base/$version/$archive" "$tmp/$archive" || fail "Falha ao baixar $archive de um host permitido." "Could not download $archive from an allowed host."
expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name { print tolower($1); exit }' "$tmp/checksums.txt")
case "$expected" in ''|*[!0-9a-f]*) fail "Archive ausente ou checksum inválido em checksums.txt." "Archive missing or invalid checksum in checksums.txt." ;; esac
[ "${#expected}" -eq 64 ] || fail "Checksum inválido para $archive." "Invalid checksum for $archive."
if [ "$hash_tool" = sha256sum ]; then actual=$(sha256sum "$tmp/$archive" | awk '{print tolower($1)}')
else actual=$(shasum -a 256 "$tmp/$archive" | awk '{print tolower($1)}'); fi
[ "$actual" = "$expected" ] || fail "SHA-256 divergente para $archive; nada foi instalado." "SHA-256 mismatch for $archive; nothing was installed."
mkdir -p "$tmp/extract" || fail "Não foi possível preparar extração." "Could not prepare extraction."
tar -xf "$tmp/$archive" -C "$tmp/extract" || fail "Não foi possível extrair o archive." "Could not extract the archive."
[ -f "$tmp/extract/gasa" ] && [ ! -L "$tmp/extract/gasa" ] || fail "O archive não contém gasa como arquivo regular na raiz." "Archive does not contain gasa as a regular file at its root."
[ -z "$(find "$tmp/extract" -type f -links +1 -print -quit)" ] || fail "Hard links não são aceitos no archive." "Hard links are not accepted in the archive."
[ "$(wc -c < "$tmp/extract/gasa")" -ge 1024 ] || fail "Executável extraído pequeno demais." "Extracted executable is too small."
chmod 755 "$tmp/extract/gasa"
"$tmp/extract/gasa" --version || fail "gasa --version falhou no binário novo; instalação não alterada." "New binary failed --version; installation was not changed."
mkdir -p "$install_dir" || fail "Não foi possível criar diretório de instalação." "Could not create installation directory."
staged=$(mktemp "$install_dir/.gasa.XXXXXX") || fail "Não foi possível preparar instalação." "Could not stage installation."
target="$install_dir/gasa"; backup="$install_dir/gasa.old"
trap 'rm -f "$staged"; rm -rf "$tmp"' EXIT HUP INT TERM
cp "$tmp/extract/gasa" "$staged" || fail "Não foi possível preparar instalação." "Could not stage installation."
chmod 755 "$staged"
rm -f "$backup"
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
