# Primeira execução e ciclo de vida dos dados

Na primeira execução interativa, o GasaCLI detecta CLIs de agentes e o Obsidian, que é opcional. A autenticação não é verificada. A seleção permite navegar com setas ou `k`/`j`, marcar com espaço, marcar todos com `a`, limpar com `n`, confirmar com Enter ou pular com Esc/`q`. Ctrl+C cancela o onboarding. Se o terminal não permitir modo raw, a seleção numerada é usada.

Claude Code e Codex aparecem selecionados por padrão quando ausentes. O plano mostra o comando exato e a origem oficial. A execução automática pergunta `Run the automatic installs now? [y/N/s]`; o padrão é `N`. Scripts oficiais `curl | sh` e `irm | iex` são exibidos como instruções e nunca executados pelo GasaCLI.

`gasa setup` fora do chat pode executar os comandos selecionados após a confirmação. `/setup` dentro do chat mostra status, seleção numerada e plano, sem executar comandos. Para instalar, rode `gasa setup` fora do chat. Para pular o onboarding inicial, use `GASA_SKIP_ONBOARDING=1` ou `--no-onboarding`; isso não impede `gasa setup`.

## Adoção do diretório de dados

Antes de escrever, `config.Ensure` e o reset usam a mesma validação de caminho: caminhos estendidos e de dispositivo no Windows, componentes symlink/reparse, diretórios protegidos e nomes reservados são recusados. Um diretório de dados novo é criado com permissões `0700`; um diretório existente e vazio pode ser adotado. Um marcador `.gasacli` válido permite completar a estrutura faltante. Sem marcador, somente o diretório padrão `os.UserConfigDir()/gasacli` pode ser adotado como legado, e apenas se `config.yaml` não vazio for estritamente válido e não houver sinais de outra aplicação (`.git`, `package.json`, `go.mod` e semelhantes).

Um `GASACLI_HOME` apontando para uma pasta existente que não satisfaz esses critérios retorna erro sem escrever nela. Escolha um caminho novo ou vazio por `GASACLI_HOME`, ou remova a pasta manualmente.

O marcador identifica os dados do GasaCLI, mas é apenas um indicador; não é uma fronteira de segurança contra outro processo ou usuário que já tenha permissão de escrita no diretório.

## Reset e uninstall

`gasa reset` mostra o plano e pede `RESET`. Apaga configuração, estado de onboarding, confiança e logs; mantém o marcador, dados de agentes/prompts/skills/plugins/notas, backups e entradas desconhecidas. `--include-backups` também apaga `backups/`, que pode conter backups do usuário e são irrecuperáveis; isso é informado no plano e no aviso.

`gasa reset --all` apaga também `agents/`, `prompts/`, `skills/`, `plugins/`, `segundo-cerebro/` e o banco local. O aviso lista essas pastas e, quando incluída, `backups/`. `gasa uninstall` equivale ao reset completo e também imprime instruções para remover o binário; não remove o executável. Se ainda não existe diretório de dados, uninstall informa que não há dados para apagar e mesmo assim mostra as instruções do binário.

`--dry-run` exibe o plano sem apagar dados. `--yes` pula a confirmação interativa. Uma entrada conhecida com tipo inesperado é mantida e gera erro final explícito, enquanto as demais operações continuam.

A remoção recursiva usa `os.Root` (Go 1.24+) para limitar as operações à árvore aberta do diretório de dados e impedir que uma troca por junction/link durante a remoção escape para outro diretório. Isso protege a fase de remoção; a inspeção do plano não é uma transação e pode ficar desatualizada se houver alterações concorrentes. Symlink de arquivo no Windows não foi testado neste ambiente. Dois resets simultâneos não usam lock; o segundo pode falhar, mas não deve corromper dados.

O plano calcula tamanho e quantidade de arquivos recursivamente, mas lista somente os dez primeiros nomes no nível superior de cada diretório. Links e reparse points conhecidos são recusados sem serem seguidos.
