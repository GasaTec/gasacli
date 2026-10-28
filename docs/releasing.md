# Procedimento de release

Este guia descreve a release dos binários e a publicação manual do pacote npm do GasaCLI. O [workflow de release](../.github/workflows/release.yml) é acionado por tags `v*`; o `NPM_TOKEN` não é usado por ele hoje.

## 1. Pré-checagens

Confirme que a árvore está revisada e que os checks passaram localmente:

```sh
gofmt -l $(git ls-files '*.go')
go vet ./...
go test -count=1 ./...
```

`gofmt -l` deve não imprimir arquivos. Confirme também que a CI da branch `main` está verde e que a versão planejada não foi publicada antes.

## 2. Criar a release no GitHub

Use uma tag semver, como `v1.2.3` ou `v1.2.3-alpha.1`, e publique-a:

```sh
git tag v1.2.3
git push origin v1.2.3
```

O push inicia o workflow de release. Ele roda `go test`, usa o GoReleaser fixado no workflow para gerar os binários, checksums e cria a GitHub Release. O GoReleaser marca automaticamente como pré-release uma versão com sufixo, como `-alpha.1`. Aguarde o job terminar antes de anunciar ou publicar o pacote npm.

## 3. Conferir assets e attestations

Na página da release, confira os seis archives (`linux`, `darwin` e `windows`, em `amd64` e `arm64`) e `checksums.txt`. O workflow também gera build provenance para cada archive e para o checksum. Com GitHub CLI instalado e autenticado, baixe os assets e verifique cada subject:

```sh
gh release download v1.2.3 --repo GasaTec/gasacli
for asset in ./*.tar.gz ./*.zip ./checksums.txt; do
  gh attestation verify "$asset" --repo GasaTec/gasacli
done
```

Consulte também o [guia de instalação](install.md), que documenta os nomes e a verificação dos archives.

Os instaladores verificam nomes e tipos permitidos antes de extrair: o binario regular exato na raiz e, como extras, LICENSE e README.md regulares.

## 4. Publicar o pacote npm manualmente

O `version` `0.0.0` em `npm/package.json` é um placeholder do repositório. A versão publicada no npm deve ser igual à tag, removendo apenas o `v` (por exemplo, a tag `v1.2.3-alpha.1` corresponde à versão npm `1.2.3-alpha.1`). Não publique o placeholder.

Faça login no registry npm antes de publicar. Dentro de `npm/`, atualize a versão sem o `v`, revise o diff e publique:

```sh
npm login
cd npm
npm pkg set version=1.2.3
npm publish --access public
```

Para uma tag alpha, substitua `1.2.3` pela versão completa, por exemplo `1.2.3-alpha.1`. O pacote atual é sem escopo (`gasacli`); `--access public` é necessário no primeiro publish de um pacote com escopo, como `@organizacao/gasacli`, para torná-lo público. Se o pacote passar a usar escopo, confira que o nome e o acesso estão configurados antes do primeiro publish.

Confirme a versão publicada e teste a instalação pelo registry:

```sh
npm view gasacli version
npm view gasacli@1.2.3 version
npx gasacli --version
```

Para validar exatamente uma versão específica com npx, pode-se também usar `npx gasacli@1.2.3 --version`. O comando deve exibir a mesma versão sem `v`.

A publicação npm é manual hoje. Nenhum job do workflow lê `NPM_TOKEN` ou publica no npm. Se a publicação automática for aprovada e um segredo `NPM_TOKEN` estiver configurado no repositório, um job futuro poderá preparar o `package.json` a partir da tag (sem `v`) e executar `npm publish --provenance --access public` com `NODE_AUTH_TOKEN` vindo do segredo. Para provenance no GitHub Actions, esse job também precisa de `id-token: write` e de configuração do registry npm no `setup-node`. Essa automação não está habilitada no workflow atual.

## 5. Smoke test dos instaladores

Execute em uma máquina descartável ou ambiente isolado, pois o instalador PowerShell inclui o diretório de instalação no PATH do usuário. Use a tag completa com `v` em `GASA_VERSION` e aponte `GASA_INSTALL_DIR` para uma pasta temporária. Exemplo no Windows:

```powershell
$tag = 'v1.2.3'
$env:GASA_VERSION = $tag
$tempRoot = (Get-Item $env:TEMP).FullName
$env:GASA_INSTALL_DIR = Join-Path $tempRoot "gasa-smoke-$tag\bin"
& .\scripts\install.ps1
& (Join-Path $env:GASA_INSTALL_DIR 'gasa.exe') --version

$env:GASA_UNINSTALL = '1'
& .\scripts\install.ps1
Remove-Item Env:GASA_UNINSTALL
Remove-Item Env:GASA_VERSION, Env:GASA_INSTALL_DIR
```

No Linux ou macOS:

```sh
tag=v1.2.3
base=$(cd -P "${TMPDIR:-/tmp}" && pwd -P)
smoke="$base/gasa-smoke/bin"
GASA_VERSION="$tag" GASA_INSTALL_DIR="$smoke" sh scripts/install.sh
"$smoke/gasa" --version
GASA_UNINSTALL=1 GASA_VERSION="$tag" GASA_INSTALL_DIR="$smoke" sh scripts/install.sh
rmdir "$smoke" "$(dirname "$smoke")" 2>/dev/null || true
```

Confirme que a versão exibida é a tag sem `v`. O teste baixa os assets publicados da release e, portanto, só funciona depois que a release está disponível. O processo de instalação e os requisitos estão detalhados no [guia de instalação](install.md).

Publique os assets da GitHub Release antes de publicar a versão correspondente do pacote npm. O `postinstall` tenta baixar esses assets; se for publicado primeiro, o pacote pode chegar antes de o checksum e o archive estarem disponíveis.

## 6. Rollback

Se a release precisar ser retirada de circulação, marque-a como pré-release ou rascunho no GitHub:

```sh
gh release edit v1.2.3 --prerelease
# ou, para ocultar a release enquanto investiga:
gh release edit v1.2.3 --draft
```

Para desaconselhar uma versão npm já publicada, sem removê-la do histórico:

```sh
npm deprecate gasacli@1.2.3 "Use a versão corrigida 1.2.4."
```

Corrija o problema e publique uma nova versão/tag. **Não apague nem reescreva uma tag publicada**: ela identifica os fontes e os artefatos daquela release e pode já estar em uso por instaladores ou consumidores.
