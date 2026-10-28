# 06 — Segundo cérebro e armazenamento local

## Princípio
GasaCLI funciona sem Obsidian. O segundo cérebro é uma pasta Markdown opcional, que pode ser criada no diretório de dados do usuário, copiada, clonada ou apontada para um vault existente.

## Comandos disponíveis
`gasa brain connect <pasta> [--yes]`, `status`, `reindex`, `search <consulta>` e `disconnect`; os mesmos comandos estão em `/brain`. A conexão indexa Markdown para SQLite FTS5 local. A busca retorna trechos citados com caminho completo, título, linhas e mtime. Hash SHA-256 permite ignorar arquivos inalterados. A reindexação é sob demanda; não há watcher.

O índice extrai tags de frontmatter e links Markdown. `.git`, `.obsidian`, `node_modules` e `vendor` são ignorados por padrão. Vaults têm limite de 10.000 arquivos e cada Markdown deve ter até 2 MiB, com no máximo 256 MiB indexados por execução. Globs extras são configuráveis pela API da biblioteca. O pacote `internal/memory` oferece `Retrieve(ctx, query, budgetTokens)` com orçamento aproximado para futura injeção no engine; o engine ainda não chama essa API.

A pasta conectada é aberta somente para leitura. Configuração e índice ficam no banco global `gasacli`; `disconnect` apaga a conexão e os dados indexados locais.
## Localização
```text
UserData/gasacli/
  config.yaml
  data/gasacli.db
  prompts/
  agents/
  skills/
  plugins/
  segundo-cerebro/
  logs/
```
Usar localização apropriada por SO; nunca dados persistentes dentro do pacote npm.

## Recuperação
L1 contexto do projeto ativo; L2 SQLite FTS5 com chunks, hashes, metadados e origem; L3 arquivos Markdown sob demanda. Indexação incremental, leitura mínima necessária e atribuição de fontes. Busca semântica opcional após benchmark.

## SQLite
Tabelas conceituais incluem runs, tasks, dispatches, events, checks, reviews, chat_sessions, messages, agent_capabilities, usage e sync_outbox. O índice do vault usa `brain_config`, `brain_files` e FTS5 `brain_chunks`. Migrações versionadas e WAL são aplicados pelo storage compartilhado.

## Escrita segura
A leitura do vault não cria nem altera arquivos na pasta selecionada, inclusive em `.obsidian`. Links simbólicos e arquivos que excedem os limites são recusados ou ignorados. A reindexação detecta mudanças pelo hash; uma edição simultânea pode ser refletida na execução seguinte.
## Segredos
Credenciais no cofre do SO, config com referências. .env apenas para desenvolvimento e nunca commitado com valores reais. Backups não exportam segredos por padrão.
