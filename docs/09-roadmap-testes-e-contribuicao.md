# 09 — Roadmap, testes, segurança e contribuição

## Etapas
M0: especificação, licença MIT, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, templates de issues/PR.
M1: monorepo TypeScript, TUI `gasa`, trust gate, discovery e adapter real.
M2: `/goal`, Run/Task/Dispatch, SQLite, prompts padrão, quality gate e retomada.
M3: segundo adapter e revisão cross-provider, worktrees e benchmarks de consumo.
M4: plugins GitHub e JEV, skills versionadas e segundo cérebro local.
M5: integração Orca opcional, hardening, CI e lançamento público.
M6: sync E2EE self-hostable, Cloud gerenciado, billing e painel.

## Critérios do MVP
Um usuário instala a CLI, abre `gasa`, autoriza a pasta, detecta um agente existente, inicia `/goal`, vê progresso, recebe mudança real, testes verificáveis e resultado DONE ou BLOCKED honesto. Sem nova chave de API exigida pela Gasatec.

## Estratégia de testes
Unitários: máquina de estados, roteamento, permissões.
Integração: fake CLIs, SQLite, eventos, resume.
E2E: dois providers reais em ambientes autorizados.
Segurança: prompt injection, vazamento de secrets, plugin malicioso, symlinks, cancelamento, shell e trust.
Cloud: conflitos, criptografia, troca/revogação de dispositivo e recuperação.

## Contribuições
Issues pequenas com critérios de aceite; branches por issue; PRs com testes, evidências e revisão. Consulte CONTRIBUTING.md, CODE_OF_CONDUCT.md e SECURITY.md. CI deve rodar lint/typecheck/test/build assim que scripts existirem. Nunca alegar que CI passou antes de executá-lo.

## Checklist de publicação
Licenças e titularidade revisadas, secret scan, dependências auditadas, canal privado de vulnerabilidade, proteção de branch, CI, documentação de instalação, termos de Cloud e privacidade quando aplicáveis. Alterar visibilidade apenas após verificação.

## Métricas
Taxa de conclusão, qualidade de código, regressões, tempo, intervenções, tokens observados, estimativas identificadas, custo por tarefa concluída e confiabilidade de retomada.

## Limitações atuais
Documentação e issues existem; não há implementação funcional da CLI ou Cloud ainda. A integração nativa com Orca depende de investigação técnica.
