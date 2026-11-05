# 01 — Pesquisa e evidências

## Método e limites

Pesquisa em 2026-11-04: leitura de README, documentação, contratos, código relacionado e ajuda da CLI instalada. Os repositórios externos foram obtidos em diretório temporário fora do projeto, sem instalar suas dependências ou executar seus scripts. Fontes externas abaixo usam commits fixos; a CLI instalada pode representar outra versão.

| Base | Identificador observado | Evidência |
| --- | --- | --- |
| GasaCLI | `d5c9a67` | `git log`, árvore limpa no início da análise e leitura de código |
| Orca upstream | `a19de980854e1c875a4555e1bd24d832c9f96657` | `git rev-parse HEAD` no clone de estudo |
| Openbot upstream | `bac07e6826d3fd6130038a6c889736386608f2ef` | `git rev-parse HEAD` no clone de estudo |
| Orca instalado | Executável da sessão resolvido e mantido durante a pesquisa | `skills get orca-cli --json`, `--help`, `terminal list`, `plugin list` |

Não houve prova ponta a ponta de execução multiagente, plugin novo, cancelamento ou renderização 3D. A pesquisa confirma superfícies de integração, não entrega sua implementação.

## GasaCLI: base existente

| Área | Código consultado | Situação |
| --- | --- | --- |
| CLI e TUI | `cmd/gasa/`, `internal/tui/` | Entrada interativa, onboarding, comandos e chat de sessões |
| Servidor local | `internal/web/server.go` | HTTP Go, assets embutidos, loopback, preferência 3199 e fallback de porta |
| Autenticação web | `internal/web/server.go`, `static/bootstrap.js` | Código de bootstrap de uso único, troca por cookie HttpOnly, validação de Host/Origin e CSP |
| Dashboard | `internal/web/static/index.html`, `app.js`, CSS | Sidebar, sessões, chat, mudanças, checks e configurações; sem escritório 3D |
| Leitura e eventos | `internal/web/api.go`, `api_runs.go` | Projetos/sessões/diff e SSE de snapshots/runs |
| Envio | `internal/web/api_send.go`, `internal/sessions/sender.go` | Retoma sessão Claude/Codex, verifica política e transmite saída; não é ainda o protocolo supervisionado proposto |
| Discovery | `internal/discovery/catalog.go` | Claude, Codex, Gemini, Cursor, OpenCode, Orca, Kimi e gh; Antigravity não consta desse catálogo |
| Execução | `internal/adapters/executor.go` | Drivers de execução próprios para Claude/Codex |
| Coordenação | `internal/orchestration/runner.go`, `internal/engine/` | Run/tarefas/eventos, checks, revisão e ciclos limitados de correção |
| Dados e confiança | `internal/storage/`, `internal/trust/`, `internal/permissions/` | Persistência local, projetos confiáveis e política fora do modelo |
| Integração | `internal/plugin/github/`, `internal/skills/` | Código existente reutilizável; SDK genérico e integração Orca nos documentos são propostas |

A documentação contém caminhos históricos como `apps/cli` e `packages/core`. Eles não descrevem a árvore Go atual. A evolução deve partir de `cmd/` e `internal/`, conforme o código e o ADR-0006.

Limites relevantes: descobrir um binário não comprova login, suporte a mensagens, cancelamento ou chat estruturado. O SSE global atual publica snapshots, não um log durável de conversas entre agentes. O envio atual usa o contexto da requisição HTTP: a arquitetura de tarefas duráveis deve separar a vida do trabalho da conexão do navegador.

## Orca: reaproveitar execução e supervisão

Fontes primárias consultadas:

- [README e organização do Orca](https://github.com/stablyai/orca/tree/a19de980854e1c875a4555e1bd24d832c9f96657).
- [Comandos de orquestração](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/docs/site/content/docs/cli/orchestration.mdx).
- [Guia de coordenação](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/skill-guides/orchestration.md).
- [Catálogo de agentes](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/src/shared/agent-launch/catalog/tui-agent.ts).
- [Manifesto de plugins](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/docs/reference/plugin-manifest.md).
- [Tipos da API do host](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/resources/plugins/types/orca-host.d.ts) e [worker](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/resources/plugins/types/orca-worker.d.ts).

O modelo upstream distingue Run, Task, Dispatch, Message e Decision gate. Um Run é um namespace durável e caixa de mensagens; não é um planejador automático. O coordenador continua necessário para decidir trabalho, tratar respostas e aplicar a política do produto.

Capacidades observadas na ajuda instalada e/ou nos contratos upstream:

| Área | Superfície | Uso proposto |
| --- | --- | --- |
| Diagnóstico | `status`, `agent-context`, `skills get` | Negociar disponibilidade e contratos por versão |
| Projetos e workspaces | `project`, `repo`, `worktree` | Mapear escritório ao projeto/host correto |
| Agentes | `agent launch`; catálogo inclui `claude`, `codex`, `antigravity`, `kimi` | Disponibilidade depende da instalação e capacidade do host |
| Trabalho supervisionado | `orchestration run-create`, `task-create`, `worker-start` | Lançar tarefa com identidade e retorno ao coordenador |
| Comunicação | `orchestration send`, `ask`, `reply`, `inbox`, `check` | Mensagens com Run/Task/Dispatch e retorno correlacionado |
| Recuperação | `worker-show`, `worker-read`, `worker-stop`, `worker-abandon`, `worker-release` | Observar, parar e conciliar sem duplicar trabalhadores |
| Decisão | `gate-create`, `gate-resolve`, `gate-list` | Bloqueios e respostas explícitas |
| Plugins | `plugin list`, `inspect`, `validate`, `types`, `logs` | Usar extensões no host e inspecionar capacidades |

Não usar `terminal send` para contornar a caixa de mensagens de um worker supervisionado. Handles são vinculados ao runtime: um ID de aba da UI não é automaticamente um handle válido. A tentativa de ler a aba Claude fornecida dessa maneira retornou `terminal_handle_stale`; a leitura das imagens foi recuperada separadamente, em modo somente leitura, a partir do journal local.

### Limites importantes de plugins

O manifesto consultado define perfis `data`, `declarative`, `sandboxed-ui` e `trusted-worker`, contribuições e permissões. `agents.list` e eventos de status ajudam a observar a equipe, mas os tipos consultados não oferecem uma API de plugins equivalente a todo o namespace de orquestração da CLI.

Os campos `agentHarnesses`, `skills` e `mcpServers` estão reservados e devem permanecer vazios nesse contrato. Não usá-los como prova de que GasaCLI já pode se registrar como harness ou importar todos os plugins automaticamente.

Há hooks de lançamento, prompt e ferramentas em versões indicadas no manifesto. O contrato registra diferenças de cobertura: certos hooks excluem entrada humana, restauração de sessão, SSH/WSL ou apenas observam ferramentas de determinados agentes. Portanto, hooks não garantem bloquear toda ação de toda CLI. A autorização anterior à execução precisa existir no gateway do escritório, com limitações externas visíveis.

O `package.json` do snapshot informa `1.4.214`, enquanto campos do manifesto indicam versões mínimas posteriores. Essa divergência impede escolher um mínimo global apenas pelo snapshot. A implementação deve verificar CLI, schema e comportamento da instalação alvo e fixar a combinação homologada.

## Openbot: referência para equipe persistente

Fontes primárias consultadas:

- [README e estrutura](https://github.com/nightly-labs/openbot/tree/bac07e6826d3fd6130038a6c889736386608f2ef).
- [Canais e comunicação](https://github.com/nightly-labs/openbot/blob/bac07e6826d3fd6130038a6c889736386608f2ef/docs/architecture/channels-and-messaging.md), especialmente a seção de canais compartilhados.
- [Contrato de canais](https://github.com/nightly-labs/openbot/blob/bac07e6826d3fd6130038a6c889736386608f2ef/packages/contracts/src/ipc-chat-channels.ts).
- [Contrato de fila](https://github.com/nightly-labs/openbot/blob/bac07e6826d3fd6130038a6c889736386608f2ef/packages/contracts/src/ipc-queue.ts).
- [Manifesto do projeto](https://github.com/nightly-labs/openbot/blob/bac07e6826d3fd6130038a6c889736386608f2ef/package.json).

Os padrões estudados são: pessoa/agente persistente; workspace e conversa próprios; canais com participantes e liderança; mensagens com IDs estáveis; filas com estados explícitos; contexto de execução distinto por canal; pausa/cancelamento/recuperação; anexos e eventos persistentes.

O contrato de fila diferencia aguardar a vez de tentar orientar um turno em andamento e admite fallback quando o provider não suporta essa orientação. Esse é um bom padrão para o GasaCLI: mostrar a capacidade real e preservar a mensagem na fila.

O Openbot usa uma aplicação desktop Electron e TypeScript, com Solid e Bun no manifesto consultado. Isso não obriga o GasaCLI a adotar outro desktop, runtime ou gerenciador de pacotes. Reproduzir a organização de equipe em Go + navegador preserva o investimento atual.

### Reutilização de código e assets

O [LICENSE do Openbot](https://github.com/nightly-labs/openbot/blob/bac07e6826d3fd6130038a6c889736386608f2ef/LICENSE) e seu manifesto declaram `PolyForm-Noncommercial-1.0.0`. O [Orca declara MIT](https://github.com/stablyai/orca/blob/a19de980854e1c875a4555e1bd24d832c9f96657/LICENSE). Esta proposta usa o Openbot como referência de comportamento e não incorpora código, componentes ou assets dele ao GasaCLI MIT. Qualquer incorporação posterior precisa avaliar os termos aplicáveis antes da distribuição.

## Imagens e referência LinkedIn

Foram abertas as duas imagens anexadas às mensagens de usuário encontradas na sessão Claude indicada. A captura do GasaCLI evidencia um problema de apresentação: textos de ferramentas e estruturas técnicas dominam a leitura. A captura do Orca documenta a direção anterior, posteriormente substituída pelo usuário.

As 12 imagens enviadas diretamente nesta conversa fornecem a referência visual do escritório 3D e a descrição de seu comportamento. O [documento 07](07-requisitos-das-imagens.md) distingue elementos visíveis, afirmações do post e requisitos propostos. O usuário determinou que o localhost mude completamente de aparência, abandonando o formato visual de Orca/IDE.

Não salvar no repositório o journal, credenciais, caminhos privados de anexos, imagens das conversas ou seu conteúdo. Esses dados não compõem a API da integração. Registrar somente os requisitos derivados; screenshots não provam suporte do Orca ou do GasaCLI às funcionalidades retratadas.
