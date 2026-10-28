# Instalação

GasaCLI está em **pre-alpha**. Os instaladores binários baixam somente assets das GitHub Releases deste repositório; não há telemetria. O GitHub recebe o endereço IP e o user-agent das requisições.

## O que é baixado e como

Os instaladores diretos e o wrapper npm baixam um archive (`.zip` no Windows, `.tar.gz` no Linux/macOS), com cerca de 3 MB (medido: 2,9 a 3,3 MB no snapshot do GoReleaser), e `checksums.txt`. Os arquivos vêm de `https://github.com/GasaTec/gasacli/releases/download/<tag>/`. O instalador compara o SHA-256 do archive com a linha correspondente em `checksums.txt` e aborta antes de instalar se não corresponder. O checksum e o archive vêm da mesma release; os instaladores não verificam attestations.

| Método | O que instala | Requisitos |
|---|---|---|
| Windows direto | `%LOCALAPPDATA%\Programs\gasacli\bin\gasa.exe` | Windows PowerShell 5.1 ou PowerShell 7+, `curl.exe` do Windows 10 1803+ |
| Linux/macOS direto | `~/.local/bin/gasa` | `curl`, `tar`, `sha256sum` ou `shasum` |
| npm global | Binário no diretório `vendor/` do pacote e shim do npm | Node.js 18+ e npm |
| npx | Pacote e binário no cache npm | Node.js 18+ e npm |
| `go install` | `$GOBIN` ou `$GOPATH/bin` | Go compatível com `go.mod` |

As instalações diretas não precisam de administrador ou `sudo`, não configuram serviços e não alteram arquivos do shell. O instalador Windows adiciona o diretório ao PATH do usuário e à sessão atual; abra um terminal novo para outros processos receberem a alteração. No Linux/macOS, ele não edita o shell; se `~/.local/bin` não estiver no PATH, imprime o comando para adicioná-lo.

Os dados do aplicativo seguem `os.UserConfigDir()`: `%AppData%\gasacli` no Windows, `~/.config/gasacli` no Linux e `~/Library/Application Support/gasacli` no macOS. Desinstalar preserva esses dados.

## Windows: PowerShell

O instalador foi exercitado em Windows PowerShell 5.1 (`powershell.exe`) e PowerShell 7 (`pwsh.exe`) com `npm/test/install-flow.test.ps1`: o teste carrega o script por dot-source e `Invoke-Expression`, e testa downloads com redirects contra um servidor local. O comando abaixo baixa a versão mutável da branch `main`; para fixar o script, substitua `main` por uma tag. Também é possível baixar o arquivo e inspecioná-lo antes de executar.

```powershell
irm https://raw.githubusercontent.com/GasaTec/gasacli/main/scripts/install.ps1 | iex
```

Para fixar uma versão ou um diretório permitido:

```powershell
$env:GASA_VERSION = 'v0.1.0-alpha.1'
$env:GASA_INSTALL_DIR = "$env:LOCALAPPDATA\Programs\gasacli\bin"
irm https://raw.githubusercontent.com/GasaTec/gasacli/main/scripts/install.ps1 | iex
```

O script usa `curl.exe` em `%SystemRoot%\System32\curl.exe`, limita redirects, tamanho e tempo de download, valida o host e a porta da URL final, e verifica o SHA-256. Não usa `Invoke-WebRequest` nem `HttpClient`.

## Política de `GASA_INSTALL_DIR`

O diretório precisa ser absoluto, local, não pode conter um componente que seja symlink/reparse point, e o último componente deve ser `bin` ou começar com `gasa` (sem diferenciar maiúsculas). Caminhos relativos como `C:tools\gasa`, caminhos UNC, prefixos estendidos como `\\?\`/`\??\`, Alternate Data Streams (`:` fora da letra da unidade) e barras invertidas inválidas são recusados. O destino também não pode estar dentro de `C:\Program Files*`, `C:\ProgramData`, `C:\Users\Public` ou `C:\Windows`.

Exemplos aceitos: `%LOCALAPPDATA%\Programs\gasacli\bin`, `%TEMP%\gasacli`, `%TEMP%\gasa-dev\bin` e `%USERPROFILE%\tools\gasa`. Exemplos recusados: `C:\`, `C:\Windows\System32`, `C:\Users\Public\gasa`, `C:tools\gasa`, `\\server\share\gasa`, ou qualquer desses destinos atravessando um junction/symlink. A mensagem começa com `Refusing install directory` e informa o motivo. A validação e as operações usam o mesmo caminho absoluto canônico.

## Linux e macOS: shell POSIX

```sh
curl -fsSL https://raw.githubusercontent.com/GasaTec/gasacli/main/scripts/install.sh | sh
```

O script recusa ambientes MINGW, MSYS e CYGWIN com `Use scripts/install.ps1 no Windows`. Ele valida o host, esquema HTTPS e porta da URL final após o download; não valida cada salto intermediário do redirect. O SHA-256 do `checksums.txt` detecta corrupção ou download incompleto.

Para definir uma tag ou diretório, use, por exemplo, `GASA_VERSION=v0.1.0-alpha.1 GASA_INSTALL_DIR="$HOME/.local/bin"`. O diretório precisa ser absoluto, terminar em `bin` ou `gasa*`, não conter `\\`, não atravessar symlink/reparse point e ficar fora de diretórios de sistema como `/usr`, `/bin`, `/etc`, `/var`, `/opt`, `/Library`, `/System`, `/Applications` e `/Users/Shared`. Pastas como `~/.cargo/bin` são aceitas. O diretório padrão `~/.local/bin` também passa pela checagem de symlink/reparse point.

## npm e Go

```sh
npm i -g gasacli
npx gasacli --version
go install github.com/GasaTec/gasacli/cmd/gasa@latest
```

O pacote npm usa a versão declarada em `npm/package.json` (ou a versão publicada do pacote) e não consulta a API de Releases nem lê variáveis `GASA_*`. Se o download de rede falhar no `postinstall`, a instalação do pacote continua e o binário pode ser tentado novamente na primeira execução. Se a verificação de integridade falhar, o postinstall sai com erro e informa que o binário não foi instalado. `go install` baixa código-fonte e módulos Go; não usa o archive ou `checksums.txt` das Releases.

## Desinstalação e PATH no Windows

Defina `GASA_UNINSTALL=1` e execute o instalador novamente, com `GASA_INSTALL_DIR` igual ao diretório instalado. A remoção apaga o binário e remove do PATH do usuário uma entrada que, após normalizar barras, `..` e barra final, corresponda ao diretório canônico do GasaCLI; não afirma que essa entrada foi criada pelo instalador. Se o valor PATH ficar vazio, o valor de registro é removido. O tipo original `REG_SZ` ou `REG_EXPAND_SZ` é preservado; `REG_MULTI_SZ` é recusado sem alteração.

## Testes do instalador

Os testes de política de diretório rodam em Windows PowerShell 5.1, PowerShell 7 e shell POSIX:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File npm/test/install-dir.test.ps1
pwsh -NoProfile -File npm/test/install-dir.test.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File npm/test/install-flow.test.ps1
pwsh -NoProfile -File npm/test/install-flow.test.ps1
```

O fluxo usa servidor HTTP local; não grava no registro real. Para o npm e o shell:

```sh
cd npm && npm test && npm pack --dry-run
sh npm/test/install-dir.test.sh
sh -n scripts/install.sh
```

## Download manual e solução de problemas

Baixe o archive e `checksums.txt` da mesma tag em [GitHub Releases](https://github.com/GasaTec/gasacli/releases), compare o SHA-256 e extraia `gasa`/`gasa.exe` para um diretório seguro no PATH. Em caso de erro de rede ou proxy, baixe os dois arquivos manualmente e valide o checksum antes de extrair. O `curl` respeita `HTTPS_PROXY`; o pacote npm não usa configuração de proxy customizada. Não substitua o checksum nem baixe de outro host.
