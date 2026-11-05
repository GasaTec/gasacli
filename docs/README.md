# Documentação do GasaCLI

## Nova direção: escritório 3D

A evolução solicitada em 2026-11-04 transforma o localhost em um escritório 3D com agentes, chat e comunicação visível. Orca fornece execução/supervisão; Openbot é referência para equipe persistente e canais. A aparência de IDE/Orca deixa de ser o alvo do localhost.

Comece pelo [índice completo do escritório 3D](escritorio-3d/README.md) e pelo [ADR-0021](adr/0021-localhost-office-and-orca-runtime.md). A documentação distingue código atual, direção de produto e funcionalidades planejadas. Não executar GasaCLI ou localhost até solicitação do usuário para testar.

Para planejar as páginas e seus comportamentos, consulte as [SDDs de telas](escritorio-3d/sdd/README.md): escritório, equipe, criar agente com bichinho, dashboard, ferramentas instaladas/instaláveis, principal/fallback e ajustes.

## Base atual e histórico

- [Estado do produto e instalação](../README.md).
- [Como funciona](01-como-funciona.md) e [comandos](02-cli-e-comandos.md).
- [Arquitetura Go](03-arquitetura-e-monorepo.md), [orquestração](04-orquestracao-e-qualidade.md) e [integrações](07-plugins-github-jev-orca.md).
- [Protocolo baseline](spec/02-protocolo-dos-agentes.md) e [segurança](spec/03-seguranca-e-permissoes.md).
- [Design anterior](design/DESIGN.md), preservado como histórico e fonte de marca/acessibilidade.
- [Contribuição](../CONTRIBUTING.md) e [política de segurança](../SECURITY.md).

Os documentos antigos e telas existentes não demonstram implementação do novo escritório. A nova proposta preserva o backend e redefine a experiência visual e sua integração.
