# 06 — Segundo cérebro e armazenamento local

## Princípio
GasaCLI funciona sem Obsidian. O segundo cérebro é uma pasta Markdown opcional, que pode ser criada no diretório de dados do usuário, copiada, clonada ou apontada para um vault existente.

## Comandos propostos
`gasa brain init`, `gasa brain connect <pasta>`, `gasa brain status`, `gasa brain reindex`.

## Localização
```text
UserData/GasaCLI/
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
Tabelas conceituais: runs, tasks, dispatches, events, checks, reviews, chat_sessions, messages, agent_capabilities, usage, memory_files, memory_chunks, sync_outbox. Migrações versionadas, foreign keys, WAL e backup consistente.

## Escrita segura
Indexar em modo leitura por padrão. Alterações no vault devem apresentar diff e pedir autorização. Excluir caminhos sensíveis, limitar arquivos grandes, lidar com symlinks e edição concorrente.

## Segredos
Credenciais no cofre do SO, config com referências. .env apenas para desenvolvimento e nunca commitado com valores reais. Backups não exportam segredos por padrão.
