# Contratos dos agentes e máquina de estados — baseline v1

## Entidades
Project{id,canonicalPath,trustState}; Run{id,projectId,goal,status,policyVersion,createdAt}; Task{id,runId,dependencies,acceptance,status}; Dispatch{id,taskId,attempt,agentId,status}; Event{id,runId,taskId?,dispatchId?,type,ts,payloadVersion}; Check{id,taskId,commitSha,command,exitCode,artifact}; Review{id,taskId,commitSha,providerFamily,verdict,findings}; Approval{id,runId,scope,decision,expiresAt}.

## Estados
Run: CREATED, PLANNING, EXECUTING, VERIFYING, REVIEWING, FIXING, NEEDS_APPROVAL, BLOCKED, DONE, CANCELLED.
Task: QUEUED, READY, RUNNING, VERIFYING, REVIEWING, FAILED, BLOCKED, DONE, CANCELLED.
Dispatch: CREATED, STARTING, RUNNING, SUCCEEDED, FAILED, TIMED_OUT, CANCELLED.

## Eventos mínimos
run.created; task.planned; task.ready; dispatch.started; worker.message; worker.progress; dispatch.finished; check.started; check.finished; review.requested; review.finished; approval.requested; approval.resolved; run.blocked; run.completed; run.cancelled.

Cada evento carrega eventId único, timestamp UTC, IDs correlacionados, versão de schema e payload sanitizado. Consumo de tokens é evento separado e idempotente por ID de origem.

## Protocolo
O core entrega ao adapter: task ID, diretório, objetivo, contexto delimitado, políticas de ferramentas, timeout e formato de saída. Adapter responde com eventos estruturados, resultado, arquivos alterados, código de saída e uso quando fornecido. Mensagens entre agentes passam pelo core; worker não atribui a si mesmo status DONE.

## Conclusão
DONE exige critérios de aceite verificados, checks exigidos aprovados, revisão independente válida para o commit/diff atual, nenhum bloqueio pendente e confirmação do core. Mudanças após review invalidam aprovação. Falhas repetidas, orçamento esgotado ou revisor indisponível geram BLOCKED, nunca loop infinito.

## Idempotência e retomada
Identificadores estáveis por tentativa, deduplicação de eventos, persistência antes de dispatch, revalidação de worktree e estado após restart, não repetir automaticamente operações não idempotentes. Cancelamento propaga ao subprocesso e registra estado.

## Isolamento
Confiar no diretório não concede permissão irrestrita. Worktrees para edições concorrentes; serializar merge; evitar ciclos de dependência e orquestração recursiva Orca↔GasaCLI.

## Testes de contrato a implementar
Duplicação/reordenação de eventos; subprocesso encerrado; queda de energia; timeout; retry; review invalidado; revisor igual ao developer; falha de build; cancelamento; limite de orçamento; concorrência e recuperação.
