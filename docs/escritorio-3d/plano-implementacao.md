# Escritório GasaCLI — Implementation Plan

> **For agentic workers:** executar as unidades abaixo com responsáveis e arquivos separados; revisar conformidade e qualidade antes de integrar. Preservar o WIP documental existente.

**Goal:** entregar o sistema documentado nas SDDs, incluindo escritório 3D, cadastro de personagens/mesas, ferramentas instaláveis, principal/fallback, comunicação supervisionada, dashboard, configurações e extensões previstas.

**Architecture:** Go mantém servidor autenticado, SQLite, política e integração por CLI pública do Orca. O frontend usa assets locais, cena 3D e páginas próprias do estúdio. Estado confirmado e eventos governam a cena; trabalho nunca depende da animação.

**Tech Stack:** Go/SQLite existentes; módulos JavaScript locais e renderização 3D; backend Orca versionado.

## Restrição preservada

Não executar `gasa`, `go run`, localhost ou testes que iniciem o produto até pedido explícito para testar. É permitido editar, inspecionar código e efetuar verificação estática sem iniciar o aplicativo. Nenhuma instalação global, login de provider ou alteração de dados reais durante o desenvolvimento.

## Contrato inicial compartilhado

- API autenticada sob `/api/office/`; projeto usa o ID já retornado por `/api/projects`.
- Agente: `id, revision, status, name, role, whenToCall, instructions, avatar, primary, fallbacks, fallbackMode, allowedCapabilities, teamIds, deskId`.
- Avatar: `familyId` (fox/cat/owl/capybara/robot), `color`, `eyes`, `accessories`.
- Principal/fallback: `toolId, model, effort`; modelo vazio significa padrão da ferramenta e não catálogo presumido.
- Snapshot: `projectId, agents, teams, messages, tasks, settings, cursor`; arrays vazios são reais, sem demonstração automática.
- Status de cadastro: draft/active/paused/archived. Criar ativo persiste mesa e evento na mesma transação; não inicia processo.
- UI envia chave idempotente e revisão; backend valida contexto, regras e conflitos.

## Unidades e responsabilidade

### 1. Domínio e persistência — worker office-domain

Arquivos: novos `internal/office/*.go` exclusivamente. Reutilizar `internal/storage` sem editar seu código. Migração aditiva com versão livre verificada. Entregar agentes, mesas, teams, conversas, intenções, eventos, configurações, tentativas e métricas derivadas, com validação, idempotência e revisões. Não executar testes nesta fase.

- [ ] Tipos e regras de cadastro/roteamento.
- [ ] CRUD transacional e eventos com isolamento por projeto.
- [ ] Mensagens/intenção/decisão, tarefas e parada persistentes sem falso resultado.
- [ ] Contratos de teste definidos em arquivos de teste, aguardando autorização para execução.

### 2. Frontend — worker office-ui

Arquivos: novos `internal/web/static/office-*`, assets locais e `index.html`; preservar frontend anterior em entrada legada. Implementar todas as páginas SDD e cena 3D real. Não editar Go.

- [ ] Shell do estúdio, escritório 3D e modo acessível.
- [ ] Wizard de bichinhos, cadastro/edição e mesa por estado confirmado.
- [ ] Equipes, conversas, tarefas, dashboard e ajustes.
- [ ] Instalação/fallback/aprovação com capacidades reais e estados de erro.

### 3. Ferramentas e runtime — worker runtime-bridge

Arquivos: novos `internal/integrations/host/*.go` e `internal/integrations/orca/*.go` exclusivamente. Não editar catálogos existentes sem coordenação.

- [ ] Discovery unificado por IDs; distinguir instalação, login e capacidades.
- [ ] Plano/job de instalação explícitos, duráveis e idempotentes.
- [ ] CLI Orca com argumentos separados, timeout/limites/recibos e observação não destrutiva.
- [ ] Coordenação, mensagens, tentativas, parada e resultado desconhecido conforme contratos públicos comprovados.

### 4. Integração — coordenador

Arquivos: `internal/web/office*.go`, integração mínima em `server.go`, documentação e composição de serviços. Inspecionar interfaces entregues e integrar sem mudar contratos antigos silenciosamente.

- [ ] Autenticação, Host/Origin, trust e escopo por projeto em todas as rotas.
- [ ] Registrar capabilities, snapshots/eventos, CRUD e controle durável.
- [ ] Revalidar ferramentas, coordenador, principal/fallback e políticas antes do trabalho.
- [ ] Evitar enviar sessão supervisionada pelo caminho antigo.
- [ ] Revisar diff, estática e testes apropriados quando autorizados.

### 5. Extensões e auditoria de entrega

- [ ] Voz por pessoa e copiloto com grants temporários e suporte efetivo, sem contratação/API implícita.
- [ ] Plugins/skills, dados de assinatura e dashboard ligados a evidência real.
- [ ] Auditar cada requisito das SDDs contra código e prova autorizada.
- [ ] Manter pendências explícitas; goal só conclui com todos os requisitos demonstrados.

## Revisão

Conferir a implementação contra `sdd/README.md` e os cenários E2E-01–12. Revisão de conformidade precede qualidade. Checks estáticos não equivalem a build, execução ou prova de provider. Nenhuma conclusão ampla baseada em fixture ou análise textual.
