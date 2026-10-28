# Checklist de governança e lançamento público

Este checklist registra tarefas e evidências necessárias. O repositório **já é público**; não altere sua visibilidade como parte deste checklist. Nenhum item abaixo afirma que uma configuração externa já está habilitada.

## Ações exclusivas do owner/maintainer — @amaralzando

- [ ] **Canal privado de vulnerabilidades:** habilitar GitHub Private vulnerability reporting nas configurações do repositório. Conferir a opção Report a vulnerability e testar que o fluxo abre um advisory privado sem publicar detalhes. Até confirmar, `SECURITY.md` deve orientar contato privado alternativo.
- [ ] **Secret scanning:** verificar disponibilidade/licença do recurso para este repositório e habilitar secret scanning e push protection quando disponíveis. Revisar alertas e remediar credenciais expostas; não copiar valores secretos para issues ou relatórios.
- [ ] **Discussions:** habilitar Discussions se o maintainer quiser receber perguntas e propostas por esse canal; criar categorias e apontar `CONTRIBUTING.md` para elas após habilitar.
- [ ] **Proteção de `main`:** configurar ruleset/branch protection: PR obrigatório, revisão do owner, checks obrigatórios após identificar workflows estáveis, resolução de conversas e restrição de force-push/deleção. Registrar checks exatos e exceções aprovadas; não presumir que a configuração esteja ativa.
- [ ] **Contato de conduta:** definir endereço/canal privado dedicado ou confirmar contato público do maintainer; atualizar `CODE_OF_CONDUCT.md` e `GOVERNANCE.md` com instruções que possam ser usadas por qualquer participante.
- [ ] **Rótulos:** criar e manter o conjunto inicial abaixo no GitHub; validar nomes contra templates e triagem real.
- [ ] **Licença e titularidade:** confirmar que o owner tem direito de publicar o código sob MIT e revisar histórico/arquivos importados. `LICENSE` declara MIT; a declaração não substitui a verificação de titularidade.
- [ ] **Visibilidade:** o repo já é público. Não mudar visibilidade sem decisão explícita do owner e revisão de segredos, dados e histórico.

## Inventários e evidências

- [ ] **Dependências Go:** gerar inventário de módulos diretos e transitivos de `go.mod`/`go.sum`, obter as licenças dos módulos selecionados e registrar nome, versão, licença, URL de origem e avisos/atribuições exigidos. Investigar casos sem licença clara; não inferir licença pelo nome do pacote.
- [ ] **Dependências npm:** inventariar dependências diretas/transitivas e pacotes de instalação usados pelo wrapper `npm/`; registrar nome, versão, licença, URL e avisos. Conferir também metadados de publicação e arquivos incluídos no pacote.
- [ ] Guardar relatórios de ferramenta e revisão humana em artefato/documentação de lançamento, incluindo data e versão analisada. Ferramenta automática é triagem; resolve manualmente identificações ausentes ou conflitantes.
- [ ] **Secrets e conteúdo privado:** revisar histórico e artefatos publicados com scanner; verificar que não há tokens, credenciais, dados pessoais ou conteúdo de vault. Revogar/rotacionar segredo exposto, não apenas removê-lo do HEAD.
- [ ] **Documentação comunitária:** conferir `GOVERNANCE.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, templates de issues/PR e instruções de instalação; substituir referências a canais ainda não habilitados.
- [ ] **CI e releases:** confirmar checks necessários, origem dos binários, checksums/attestations e instruções de reporte antes de chamar uma release de pronta. Registrar resultado real e SHA/tag revisados.

## Conjunto inicial de labels

- [ ] `bug` — comportamento incorreto reproduzível.
- [ ] `enhancement` — melhoria ou capacidade nova.
- [ ] `documentation` — documentação e exemplos.
- [ ] `security` — assunto de segurança não sensível; detalhes de vulnerabilidade não devem ir em issue pública.
- [ ] `good first issue` — tarefa pequena com contexto suficiente para nova contribuição.
- [ ] `help wanted` — contribuição externa desejada.
- [ ] `needs triage` — ainda sem escopo/owner definidos.
- [ ] `blocked` — dependência ou impedimento documentado.
- [ ] `duplicate`, `wontfix` — encerramento com explicação e referência quando aplicável.
- [ ] `area:cli`, `area:cloud`, `area:docs`, `area:security` — domínio principal.

## Registro por lançamento

Para cada lançamento público, registrar data, tag/SHA, inventário de dependências e licenças correspondente, resultado de secret scan, checks de CI e pendências owner-only. Marcar como concluído apenas com evidência verificável; itens não checados permanecem abertos.
