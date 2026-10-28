# Instalação

GasaCLI esta em pre-alpha. Os instaladores baixam assets da GitHub Releases; sem GASA_VERSION tambem consultam api.github.com/repos/GasaTec/gasacli/releases para obter a versao. Nao ha telemetria; o GitHub recebe IP e user-agent.

## O que é baixado e como

Os instaladores diretos e o wrapper npm baixam um archive e checksums.txt. Antes de extrair, exigem exatamente um binario regular com nome exato na raiz e permitem apenas LICENSE e README.md como extras regulares. O SHA-256 e comparado com a linha correspondente em checksums.txt. Os dois assets vem da mesma release; attestations nao sao verificadas.

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

O script usa `curl.exe` em `%SystemRoot%\System32\curl.exe`, limita redirects, tamanho e tempo de download, valida HTTPS, host e porta antes de cada salto de redirect, e verifica o SHA-256. Não usa `Invoke-WebRequest` nem `HttpClient`.

## Política de `GASA_INSTALL_DIR`

O diretório precisa ser absoluto, local, não pode conter um componente que seja symlink/reparse point, e o último componente deve ser `bin` ou começar com `gasa` (sem diferenciar maiúsculas). Caminhos relativos como `C:tools\gasa`, caminhos UNC, prefixos estendidos como `\\?\`/`\??\`, Alternate Data Streams (`:` fora da letra da unidade) e barras invertidas inválidas são recusados. O destino também não pode estar dentro de `C:\Program Files*`, `C:\ProgramData`, `C:\Users\Public` ou `C:\Windows`.

Exemplos aceitos incluem %LOCALAPPDATA%\Programs\gasacli\bin e %TEMP%\gasa-dev\bin. Ambos recusam ; % * ? aspas < > |, C0/C1, DEL, marcas direcionais e separadores Unicode; o shell tambem recusa $, crase, barra invertida e dois-pontos.

Se uma atualização for interrompida, a próxima execução remove temporários do instalador e restaura o backup `.old` quando o executável estiver ausente. Atualizações usam nomes de backup únicos; backups bloqueados pelo sistema são mantidos e reportados.

## Linux e macOS: shell POSIX

```sh
curl -fsSL https://raw.githubusercontent.com/GasaTec/gasacli/main/scripts/install.sh | sh
```

O script recusa ambientes MINGW, MSYS e CYGWIN com `Use scripts/install.ps1 no Windows`. Ele valida HTTPS, host e porta antes de requisitar cada salto de redirect. O SHA-256 do `checksums.txt` detecta corrupção ou download incompleto.

Para definir uma tag ou diretório, use, por exemplo, `GASA_VERSION=v0.1.0-alpha.1 GASA_INSTALL_DIR="$HOME/.local/bin"`. O diretório precisa ser absoluto, terminar em `bin` ou `gasa*`, não conter `\\`, não atravessar symlink/reparse point e ficar fora de diretórios de sistema como `/usr`, `/bin`, `/etc`, `/var`, `/opt`, `/Library`, `/System`, `/Applications` e `/Users/Shared`. Pastas como `~/.cargo/bin` são aceitas. O diretório padrão `~/.local/bin` também passa pela checagem de symlink/reparse point.

## npm e Go

```sh
npm i -g gasacli
npx gasacli --version
go install github.com/GasaTec/gasacli/cmd/gasa@latest
```

O pacote npm usa sua versao declarada e nao consulta a API de Releases nem le GASA_*. Falhas de rede, inclusive timeout total ou idle, geram aviso no postinstall e sao tentadas novamente na primeira execucao. HTTP 403, falha de integridade ou extracao fazem npm install falhar.

No Windows, o instalador avisa quando a entrada resultante do PATH ultrapassa 2048 caracteres. Ele só bloqueia a gravação quando se aproxima do limite da variável de ambiente, em 32000 caracteres.

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

## Recuperacao, interrupcao e concorrencia

Shell e PowerShell usam .gasa-install.lock por diretorio; o uninstall tambem adquire o lock. Um lock com mais de 30 minutos e considerado preso mesmo se o processo estiver vivo; PID ausente tambem permite reassumir. Lock shell sem owner so e orfao apos 60 segundos. Apenas quem adquiriu o lock o remove.
Os instaladores recusam controles C0/C1, DEL, marcas direcionais e separadores de linha Unicode. Os tres parsers removem BOM inicial, aceitam espaco ASCII, tab, * antes do nome e hashes repetidos identicos; hash em maiusculas e normalizado. NBSP e U+3000 nao sao separadores aceitos; .sig e .sbom nao correspondem ao archive.
O shell limpa arquivos temporarios em SIGINT, SIGTERM ou SIGHUP. Esses handlers nao rodam em SIGKILL, encerramento forcado ou queda de energia; uma nova execucao remove staging orfao e tenta restaurar backup se o binario estiver ausente. No PowerShell, falha ao limpar staging ou restaurar backup interrompe a recuperacao; no shell, uma falha pode deixar arquivos para a proxima tentativa.
A recuperacao e best effort e nao restaura gravacoes perdidas por falha de energia.
