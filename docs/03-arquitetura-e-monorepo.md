# 03 — Arquitetura e monorepo

## Stack proposta
TypeScript/Node.js; SQLite local com WAL e FTS5; subprocessos para CLIs externas; sem Redis/Celery obrigatórios. Bibliotecas concretas serão definidas com testes de compatibilidade.

## Estrutura planejada
```text
apps/cli/
apps/cloud-api/
apps/cloud-web/
packages/core/
packages/tui/
packages/discovery/
packages/agent-adapters/
packages/orchestrator/
packages/decision-engine/
packages/model-router/
packages/quality/
packages/prompts/
packages/memory/
packages/plugin-sdk/
packages/plugins-github/
packages/plugins-jev/
packages/plugins-orca/
packages/sync-protocol/
docs/
```

## Responsabilidades
Core: contratos, políticas e eventos. Discovery: capacidades verificadas. Adapters: execução, streaming e cancelamento por CLI. Orchestrator: DAG, workers e checkpoints. Router: políticas fixas ou decisões JEV. Quality: testes/review. Memory: Markdown e indexação. Plugins: integrações opcionais. Cloud: serviço gerenciado de sincronização.

## Contratos
Run(id, goal, state, budget); Task(id, runId, dependencies, criteria); Dispatch(id, taskId, attempt, agent); WorkerEvent(id, dispatchId, type, timestamp); Check(id, commit, command, exitCode); Review(id, commit, reviewerProvider, verdict); Approval(id, scope, status).

Persistir eventos com versão de schema e IDs correlacionados. Não registrar segredos. Cada adapter informa suas capacidades reais, sem assumir que todas as CLIs suportam os mesmos recursos.

## Invariantes
Cloud opcional; permissão fora do controle do modelo; sem recursão Orca↔GasaCLI; revisão de diff invalidada por novo commit; execução concorrente isolada; nenhuma conclusão sem verificação.
