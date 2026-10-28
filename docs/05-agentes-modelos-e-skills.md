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
`gasa skills` lista manifests `SKILL.md` de fontes locais: `~/.claude/skills`, `~/.claude/plugins`, `~/.codex/skills`, `~/.agents/skills`, skills do projeto e `<gasacli>/skills`. `show` exibe origem, versão, hash e caminho. A instalação de uma pasta local mostra primeiro o hash e a lista exata de arquivos; o usuário confirma em uma segunda chamada com `--confirm <sha256>`. Remoção exige `--yes`; as operações consultam a permissão `plugin_install`. O conteúdo nunca é executado.

URLs Git HTTPS exigem o commit completo no fragmento, aprovação de rede explícita em `--fetch`, prévia do hash e confirmação posterior. `recommendations` lista Ponytail 5 e Caveman como sugestões com motivos; não instala nenhum item. A tabela `skill_run_usage` e `skills.RecordActivation` oferecem um hook idempotente para associar skills a runs quando o engine passar a chamá-lo.
## Medição
Registrar tokens quando fornecidos pelas CLIs, estimativas explicitamente marcadas e custo apenas quando mensurável. Respeitar limites e termos de cada provedor. Se só houver um provedor, não fingir revisão independente.
