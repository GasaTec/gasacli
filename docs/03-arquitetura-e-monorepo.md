# 03 — Arquitetura e monorepo

## Stack aprovada
**Go** para CLI, TUI, orquestrador, discovery, adapters, plugins e persistência. **Rust** para componentes isolados somente quando benchmarks justificarem. **SQLite** com WAL/FTS5 local. **TypeScript** para o dashboard Cloud. API Cloud: Go preferencial, ainda a validar. Sem Redis/Celery obrigatórios. Veja [ADR-0006](adr/0006-go-rust-stack.md).

## Estrutura planejada
```text
cmd/gasa/
internal/tui/
internal/core/
internal/orchestrator/
internal/discovery/
internal/adapters/
internal/routing/
internal/quality/
internal/storage/
internal/memory/
internal/plugins/
pkg/pluginprotocol/
rust/
apps/cloud-api/
apps/cloud-web/
docs/
```

## Responsabilidades
Core: contratos, políticas e eventos. Discovery: capacidades verificadas. Adapters: execução, streaming e cancelamento por CLI. Orchestrator: DAG, workers e checkpoints. Router: políticas fixas ou decisões JEV. Quality: testes/review. Memory: Markdown e indexação. Plugins: integrações opcionais. Cloud: serviço gerenciado de sincronização.

## Contratos
Run(id, goal, state, budget); Task(id, runId, dependencies, criteria); Dispatch(id, taskId, attempt, agent); WorkerEvent(id, dispatchId, type, timestamp); Check(id, commit, command, exitCode); Review(id, commit, reviewerProvider, verdict); Approval(id, scope, status).

Persistir eventos com versão de schema e IDs correlacionados. Não registrar segredos. Cada adapter informa suas capacidades reais, sem assumir que todas as CLIs suportam os mesmos recursos.

## Invariantes
Cloud opcional; permissão fora do controle do modelo; sem recursão Orca↔GasaCLI; revisão de diff invalidada por novo commit; execução concorrente isolada; nenhuma conclusão sem verificação.
