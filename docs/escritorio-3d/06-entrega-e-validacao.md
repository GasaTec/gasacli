# 06 — Entrega e validação

## Restrição de execução desta sessão

O usuário determinou: não executar o GasaCLI nem abrir o localhost até pedir para testar. Esta entrega é documental. Não rodar `gasa`, `go run ./cmd/gasa`, binário compilado, servidor, testes E2E ou suites que iniciem o aplicativo. Checks descritos abaixo são trabalho futuro condicionado a essa autorização para testar.

Esta documentação não instala dependências, migra banco ou executa agentes. Revisão de texto, links locais e diff pode ser feita sem iniciar o produto.

## P0 — Base e viabilidade

Entrada: código atual, ADRs, snapshots estudados e referências visuais recebidas.

Entregas: esta especificação; inventário de capacidades; decisão sobre responsabilidades e lista de limitações. Antes de implementar execução, confirmar versão instalada, identidade autorizada do coordenador e caminho público para mensagens da UI alcançarem o Run correto.

Critério de saída: fluxo tecnicamente demonstrável por contrato, com recursos desconhecidos bloqueados. Se faltar API upstream, registrar a lacuna e definir extensão suportada; não contornar identidade do host.

## P1 — Contrato do adaptador Orca

Áreas previstas: `internal/integrations/orca/`, integração com `internal/adapters/` e configuração existente.

Entregas:

- Resolução de executável/host, JSON, timeouts, erros tipados e catálogo de capacidades.
- Leitura de projetos/workspaces/plugins e vínculo de IDs sem lançar processos de agentes.
- Operações de Run/Task/Dispatch/mensagens por interfaces pequenas e testáveis.
- Identidade de coordenador comprovada; nenhum scheduler duplicado.
- Recibos, estado desconhecido e reconciliação após falha de transporte.

Validação futura: subprocesso fake para versões incompatíveis, payload truncado, exit não zero com JSON, `ok:false`, perda de resposta e handles expirados. Contrato com Orca real só em ambiente de teste autorizado.

Saída: nunca executar uma operação que a versão/host não confirma suportar. Registrar fixture sanitizada com origem e versão.

## P2 — Identidades, canais e persistência

Áreas previstas: `internal/storage/`, serviço de aplicação do escritório, `internal/web/`.

Entregas: integrante persistente, papel e configurações, canal e membros, equipes fixas, contexto por projeto, mensagens sequenciadas, intenção/revisão/decisão, outbox e mapeamento de runtime. Criar identidade não lança CLI.

Detalhamento nas [SDDs de telas](sdd/README.md): wizard com bichinho, principal/fallback e permissões; vínculo agente/escritório/mesa em transação; rascunho, revisão, pausa e arquivo; catálogo de ferramentas com plano/job de instalação explícito. Reutilizar discovery/onboarding após separar apresentação interativa de serviços de aplicação; não executar o wizard TUI por requisição HTTP.

Validação futura: migração aditiva sobre cópia descartável do banco; preservação de runs antigos; rollback de transação; duplicação de mensagem; isolamento entre projetos; sessão distinta por contexto; não perda de fila em restart.

Saída: modelo de dados independente da cena; excluir preferências visuais não perde tarefas ou mensagens.

## P3 — Chat e coordenação operacionais

Áreas previstas: serviço de aplicação, `internal/orchestration/`, política, adaptador e rotas novas.

Entregas: conversar com time/coordenador/pessoa; antes de executar, registrar intenção e ciência/decisão do coordenador; fila por contexto; perguntas e respostas; aprovações humanas; bloqueio de conflito; três níveis de autonomia; parar tarefa e equipe.

As rotas antigas de envio não podem contornar coordenação de sessões supervisionadas. Ao vincular uma sessão ao escritório, identificar seu dono/backend e encaminhar pelo gateway ou recusar o envio direto. Sessões externas ficam identificadas e não recebem promessa de controle total.

Implementar principal/fallback por revisão de agente conforme [SDD-03](sdd/03-ferramentas-instalacao-e-fallback.md): alternativa ordenada, decisão/ciência do coordenador, contexto compatível, limite de tentativas e bloqueio em resultado desconhecido. Troca não altera backend do run, permissões nem personagem.

Validação futura: traço com `intent.recorded < coordination.allowed < dispatch.accepted`; ausência/negação/expiração de decisão impede execução; concorrência não duplica dispatch; mensagem para agente ocupado preserva fila; parada parcial é visível; fechar navegador não cancela nem relança tarefa.

Saída: um pedido direto chega ao agente correto, com coordenação anterior comprovada e resposta no mesmo chat.

## P4 — Novo frontend e escritório 3D

Áreas previstas: `internal/web/static/`, pipeline de assets e módulo de cena isolado.

Entregas: substituir a composição principal de IDE por estúdio 3D; criar cena e avatares próprios; mesas/monitores/telefones/reunião/café; chat integrado; editor de agente; modo de equipe em cartões; projeção de eventos; acessibilidade e qualidade adaptativa.

Começar a apresentação com fixtures sintéticas identificadas, sem chamar modelos. Fixtures nunca são fallback disfarçado de dados reais. Depois conectar o mesmo estado visual ao gateway validado em P3. Esse trabalho visual pode ocorrer antes do teste real do backend, mas não prova escritório funcional completo.

Validação futura: componente/projeção sem GPU; e2e sintético autorizado; screenshot dos estados acordados; teclado, reduced motion, zoom, mobile, perda de WebGL e asset ausente. Medir RAM/GPU/fps/latência em hardware registrado.

Saída: estados visuais correspondem aos eventos reais; objetos abrem o contexto correto; todas as ações essenciais funcionam sem o canvas.

## P5 — Plugins e cobertura de providers

Entregas: inventário de plugins do host, ações suportadas, acesso aos painéis existentes quando necessário e plugin GasaCLI opcional somente se seu contrato for suficiente. Não construir outro marketplace ou loader.

Claude e Codex são o primeiro par de validação heterogênea por já existir base local. Antigravity e Kimi entram com teste próprio por capacidade; outros são adicionados progressivamente. Não usar nome no catálogo como prova de execução/chat/cancelamento.

Validação futura: contrato de manifesto da versão alvo; consentimento; erro de plugin; revogação; incompatibilidade; ausência de API; diferenças entre terminal e chat; execução, mensagens, recuperação e parada por provider.

Saída: matriz de compatibilidade publicada com evidência por célula. “100% integrado” aplica-se à matriz acordada e comprovada, não a todo plugin arbitrário do ecossistema.

## P6 — Uso, assinaturas e analytics

Entregas: uso informado por provider/Orca, tarefas, duração, arquivos e agregação por integrante/modelo/dia. Mostrar janelas e limites apenas se reportados; indicar fonte, atualização e ausência. Não contratar API paga para completar uma métrica faltante.

O Dashboard é página própria do estúdio, conforme [SDD-04](sdd/04-dashboard-e-operacao.md). Agregar tarefa e tentativas separadamente e atribuir uso à ferramenta efetiva, inclusive quando fallback, sem duplicar pedido ou conclusão.

Validação futura: deduplicação após replay; mudança de timezone; tarefa cancelada; cache tokens; provider sem quota; tempo concorrente sem soma enganosa; custo equivalente separado da cobrança real.

Saída: nenhuma porcentagem de assinatura inventada; nenhuma estimativa apresentada como fatura.

## P7 — Voz e copiloto de tela

Subentregas separadas: voz local por agente e prévia; integração opcional de voz externa; observação consentida de tela; controle temporário via host compatível; revogação e overlay desktop se suportados.

Cada uma requer desenho e teste próprios antes de habilitar. F12 global, expiração de dez minutos e parada precisam de enforcement fora da renderização web. Não presumir inclusão de geração de mídia ou voz nos planos de código.

Saída: controles voluntários, capacidade comprovada, permissão temporária revogável e ausência de cobrança/compartilhamento implícitos.

## Matriz de aceite da integração

Status inicial de todas as linhas: **planejado**, sem prova funcional nesta entrega.

| Capacidade | Evidência necessária para aprovar |
| --- | --- |
| Inicialização web | Somente depois de autorizado, abrir escritório no localhost com bootstrap seguro |
| Discovery | Distinguir binário, autenticação, host e capacidades |
| Coordenador | Identidade autorizada e Run correto, sem simulação de contexto |
| Claude + Codex | Pedido, execução, mensagens, revisão e retomada em ambos |
| Antigravity | Mesmos fluxos homologados ou limitações explicitamente bloqueadas |
| Kimi | Mesmos fluxos homologados ou limitações explicitamente bloqueadas |
| Chat direto | Evento de coordenação anterior ao dispatch e resposta correlacionada |
| Agente → agente | Entrega e resposta visíveis sem trabalho fora do escopo |
| Equipe fixa | Subtarefas com papéis e dependências, sem duplicação de trabalho |
| Aprovação | Pedido, resolução, negação, expiração e vínculo com escopo/revisão |
| Parada global | Nenhum novo dispatch; retorno por worker e desconhecido explícito |
| Recuperação | Restart/perda de conexão sem prompt ou worker duplicado |
| Plugins | Funcionalidades acordadas via API suportada e consentimento vigente |
| Escritório | Cena nova com mesa, monitor, telefone, reunião e estados correlacionados |
| Acessibilidade | Operação essencial em lista/teclado e sem WebGL |
| Dados de assinatura | Fonte e horário, sem estimativas confundidas com limites reais |

## Estratégia de testes futura

1. Unitários e contratos com fakes: política, intenção, fila, recibos e projeção visual.
2. Integração local descartável: armazenamento e servidor autenticado com dados sintéticos.
3. Orca real autorizado: projeto/identidades de QA, sem tocar sessões pessoais ativas.
4. Providers reais autorizados: tarefas pequenas e limitadas, identificando custo/assinatura e evidência.
5. Browser/3D autorizado: estados sintéticos e depois traço real; screenshots e métricas.

Cobrir mensagens duplicadas/reordenadas, falha entre persistência e envio, timeout com aceitação, restart de host, sessão expirada, escopo alterado, negação humana, provider ocupado e decisão concorrente. Nenhum teste precisa editar credenciais reais ou limpar o perfil do usuário.

Quando houver autorização para rodar checks, o CI existente usa formatação Go, `go vet ./...`, `go test -count=1 ./...`, build de `./cmd/gasa`, testes npm e instaladores por plataforma. Inspecionar antes quais testes iniciam executáveis; não disparar essas suites durante a restrição atual. O frontend futuro deve adicionar seus checks reais ao workflow, sem inventar scripts ainda ausentes.

## Migração, rollout e limites de prova

- Novo escritório sob opção explícita durante desenvolvimento, depois como tela principal quando aprovado.
- Preservar dados e rotas antigas enquanto houver consumidores; remover o layout antigo não exige apagar histórico.
- Migrações aditivas em cópia antes de dados reais; backup e reversão documentados por versão.
- Feature flag não deve permitir duas autoridades executando o mesmo run.
- Demo sintética comprova apresentação; teste com fake comprova contrato local; integração real comprova a combinação de versões testada.
- Não declarar CI, release ou compatibilidade universal com base na documentação ou num único provider.

## Pendências concretas antes de programar a execução

1. Homologar versão/contrato do Orca e identidade do coordenador acessível à ponte Go.
2. Verificar mensagens não destrutivas de observação e controle de worker via superfície pública.
3. Comprovar enforcement de política por provider, especialmente nos caminhos de terminal.
4. Fixar dependência/build da cena e produzir assets próprios com licença conhecida.
5. Obter pedido do usuário antes de executar GasaCLI, abrir localhost ou testar o produto.
