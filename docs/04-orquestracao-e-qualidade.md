# 04 — Orquestração, prompts e qualidade

## Máquina de estados
TRUST_PENDING → PLANNING → EXECUTING → VERIFYING → REVIEWING → DONE.
Transições alternativas: FIXING (volta a VERIFYING), NEEDS_APPROVAL, BLOCKED e CANCELLED.

## Papéis
Orchestrator: interpretar objetivo, planejar DAG, distribuir, supervisionar, gerenciar orçamento e verificar aceite.
Developer: implementar, documentar diff e executar testes pertinentes.
TL/Reviewer: inspecionar diff e código, segurança, arquitetura e testes; usar provedor/família diferente do autor quando revisão independente for exigida.
Tester: testes automatizados e casos de regressão.
Researcher: pesquisa delimitada, com fontes e contexto mínimo.

## Contrato de prompt
Cada prompt versionado define objetivo, escopo, ferramentas autorizadas, contexto necessário, formato estruturado de saída, critérios de conclusão e escalonamento. Templates são editáveis globalmente ou por projeto, sem sobrescrever customizações em updates.

## Exemplo /goal login
Critérios: rota renderiza, validação e estados de erro/loading, acessibilidade, nenhum segredo hardcoded, testes pertinentes, build e revisão do diff atual. O orquestrador não confia em "terminei" do Developer; valida evidências.

## Quality gate
1. Validar critérios contra arquivos e resultados observáveis.
2. Rodar comandos de teste/lint/build existentes e relevantes; não inventar comandos.
3. Enviar diff para revisor independente disponível.
4. Em falhas, corrigir e repetir verificações; nova mudança invalida revisão anterior.
5. Ao exceder tentativas, tempo, orçamento ou faltar ferramenta, reportar BLOCKED.
6. DONE inclui evidências, checks, versão revisada, agentes e consumo.

## Economia
Single-agent para tarefas pequenas; paralelismo apenas quando útil; cache de contexto; resumos estruturados; não reenviar vault inteiro; limite de retries. Avaliar sucesso e custo por tarefa concluída, não apenas tokens isolados.
