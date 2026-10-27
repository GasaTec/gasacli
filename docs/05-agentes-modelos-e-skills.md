# 05 — Descoberta, modelos, roteamento e skills

## Descoberta
Procurar executáveis no PATH e locais conhecidos. Verificar versões e interfaces documentadas sem ler credenciais. Estados: NOT_INSTALLED, INSTALLED, READY, AUTH_REQUIRED, UNSUPPORTED, ERROR.

## Adapters planejados
Claude Code, Codex CLI, Gemini CLI, Cursor CLI, OpenCode e outros. Cada adapter anuncia capacidade de execução não interativa, modelos selecionáveis, sessão, streaming, permissões, cancelamento, uso e retomada. A implementação deve validar cada versão.

## Dois roteamentos
- fixed: config.yaml determina orchestrator, developer e reviewer, com fallbacks autorizados.
- jev: plugin opcional recomenda agentes conforme tarefa, risco, capacidades e orçamento. Políticas duras prevalecem.

```yaml
version: 1
routing: { strategy: fixed }
agents:
  orchestrator: { cli: claude, model: opus }
  developer: { cli: codex, reasoning: high }
  reviewer: { cli: claude, model: sonnet }
quality:
  require_tests: true
  require_build: true
  require_independent_review: true
orchestration:
  max_retries: 3
```
Identificadores são ilustrativos e devem ser resolvidos pelos adapters.

## Skills e microagentes
Templates padrão para orquestrador, TL, backend, frontend, tester e researcher. Ponytail, Caveman e outras skills entram somente após validar origem, licença, permissões e desempenho. Pedir consentimento de instalação, fixar versão e permitir rollback. Nunca executar instruções de terceiros sem validação.

## Medição
Registrar tokens quando fornecidos pelas CLIs, estimativas explicitamente marcadas e custo apenas quando mensurável. Respeitar limites e termos de cada provedor. Se só houver um provedor, não fingir revisão independente.
