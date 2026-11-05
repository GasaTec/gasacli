# 02 — Produto e fluxos

## Experiência principal

O usuário abre o localhost e entra no seu escritório de agentes. A tela mostra quem está disponível, quem executa cada tarefa, quem aguarda resposta e quais conversas estão acontecendo. O usuário pode enviar um objetivo ao orquestrador, conversar com um agente específico ou acompanhar um canal de trabalho.

O escritório é uma interface de operação real. Cada pessoa representa uma identidade persistente, com papel, provider, configurações e contexto próprios. Um processo encerrado não apaga essa identidade; uma pessoa desenhada na tela não implica que há processo ou sessão ativa.

O [mapa de telas SDD-01](sdd/01-mapa-de-telas.md) detalha as páginas e painéis. O usuário cria cada integrante escolhendo um bichinho, função, IA principal e fallbacks; ao salvar o cadastro ativo, personagem e mesa aparecem no escritório sem iniciar trabalho. A [SDD-02](sdd/02-criar-e-gerenciar-agentes.md) especifica esse ciclo; a [SDD-03](sdd/03-ferramentas-instalacao-e-fallback.md) detalha detecção na máquina, instalação e troca de ferramenta.

## Organização inspirada no Openbot

| Entidade | Significado no GasaCLI |
| --- | --- |
| Escritório | Equipe do projeto selecionado, incluindo coordenador e especialistas |
| Integrante | Identidade persistente com nome, avatar, papel, instruções e agente/provider |
| Mesa | Posição visual atribuída ao integrante; não é um diretório de trabalho |
| Conversa direta | Histórico do usuário com um integrante, associado ao projeto |
| Canal | Conversa de uma equipe sobre um objetivo, com coordenador e participantes |
| Tarefa | Unidade de trabalho com responsável, escopo, dependências e critérios |
| Sessão | Conversa técnica do provider, associada explicitamente ao contexto de execução |
| Dispatch | Tentativa concreta de executar uma tarefa em uma sessão/worker |
| Artefato | Diff, arquivo, relatório ou evidência entregue por uma tarefa |

Um integrante pode atuar em vários canais, mas seus contextos de execução precisam de identificação própria. Não misturar conversa de outro projeto ou reutilizar uma sessão ativa concorrente para duas tarefas.

## Layout: escritório como aplicativo

- Cena 3D como área principal, com mesas, computadores, personagens, mesa de reunião e café.
- Barra superior compacta: projeto, conexão, autonomia, notificações e botão de parar equipe.
- Compositor inferior integrado à cena, com destinatário “Time”, coordenador, canal ou pessoa selecionada.
- Clique no avatar, mesa ou monitor abre conversa e atividade em painel sobreposto ou gaveta.
- Controles compactos de Escritório, Equipe, Tarefas, Dados e Ajustes; canais ficam no seletor do chat.
- Detalhes técnicos, diff e checks abrem sob demanda, sem painel permanente ocupando o escritório.

Não usar a sidebar de projetos/sessões, abas de terminal e painel de diff do Orca como composição principal. Navegação, cartões e diálogos devem pertencer à identidade do estúdio GasaCLI. Em tela estreita, cena e conversa podem alternar; compositor e aprovações continuam acessíveis sem WebGL.

## Fluxo A — Objetivo ao orquestrador

1. Usuário seleciona um projeto confiável e envia o objetivo.
2. Coordenador apresenta a decomposição, responsáveis e conflitos relevantes.
3. Backend verifica política, capacidades e contexto do projeto antes de lançar trabalho.
4. Orca cria ou associa Run/Task/Dispatch e supervisiona os workers.
5. Escritório exibe eventos de atribuição, perguntas, progresso, revisão e bloqueios.
6. Resultados passam pelos checks e pela revisão previstos na política.
7. Usuário recebe o resultado com evidências e limitações, e pode abrir o diff ou continuar a conversa.

O coordenador é um papel escolhido para um agente disponível, não um modelo novo chamado “GasaCLI”. O produto fornece roteamento, contratos e política. O Orca fornece execução e supervisão; o coordenador toma as decisões de trabalho dentro desses limites.

## Fluxo B — Mensagem direta ao especialista

Exemplo: usuário clica em Ana, desenvolvedora usando Codex, e pede uma alteração no login.

1. Mensagem é salva com destinatário Ana e intenção ainda pendente.
2. Interface informa “Aguardando coordenação”.
3. Gateway comunica ao coordenador o pedido, o escopo e o contexto relevante.
4. Coordenador verifica conflitos, tarefa em andamento, permissões e prioridade.
5. Backend registra a decisão, reserva o escopo e só então libera a execução.
6. Ana recebe a tarefa autorizada; o usuário continua conversando no mesmo chat.
7. Perguntas, mudança de escopo e resultado ficam visíveis ao coordenador e na atividade do escritório.

Esse registro ocorre antes de entregar um prompt com capacidade de executar trabalho. Não depender de um texto no prompt pedindo ao agente que “avise depois”. O usuário conversa diretamente com a pessoa, mas o produto mantém a coordenação.

Uma mensagem puramente informativa também é registrada para ciência, sem criar alteração de código. Ela não concede novas ferramentas. Se uma pergunta exigir ferramentas de execução, segue o fluxo de tarefa autorizado.

## Fluxo C — Conversa entre agentes

Um desenvolvedor pede ao revisor uma avaliação. A UI mostra origem, destino, tarefa e estado da entrega. O pedido passa pelo coordenador se atribuir trabalho ou alterar o escopo de alguém. Uma resposta informativa dentro de tarefa já autorizada não exige uma nova decisão do modelo para cada frase.

Balões e conexões representam mensagens persistidas e seus recibos, nunca conversas inventadas. Pode-se abrir a mensagem completa no histórico; animações não são o único modo de acompanhar a equipe.

## Fluxo D — Agente ocupado

Novas mensagens entram em fila por contexto de execução. A UI mostra posição, destinatário e se há dependência de coordenação. Orientação durante um turno só aparece quando a instalação demonstra suporte; nos demais casos, a mensagem aguarda o próximo turno seguro. Uma tentativa ambígua não gera reenvio automático como nova mensagem.

## Fluxo E — Aprovação, parada e recuperação

Pedidos sensíveis exibem ação, alvo, escopo e consequência. Resposta do coordenador não substitui consentimento humano exigido pela política. Se negar, expirar ou perder a conexão com o host, a tarefa permanece bloqueada e o agente recebe esse resultado.

“Parar tarefa” solicita parada pelo Dispatch correto. A tela distingue parada solicitada, confirmada e resultado desconhecido. Fechar a aba do navegador não cancela uma tarefa durável; interromper a tarefa exige ação explícita e confirmação de estado do backend.

## Capacidades e disponibilidade

Cada integrante mostra separadamente:

| Dimensão | Exemplos |
| --- | --- |
| Identidade | Ana, desenvolvedora, avatar e papel |
| Provider | Codex, Claude Code, Antigravity, Kimi ou outro informado pelo host |
| Disponibilidade | Ausente, instalado, autenticação desconhecida, pronto, indisponível |
| Interface | Chat estruturado, terminal ou somente observação |
| Execução | Sem tarefa, fila, trabalhando, aguardando decisão, bloqueado, finalizado |
| Evidência | Provider/Orca/GasaCLI, horário e estado de sincronização |

Um agente detectado sem driver comprovado pode aparecer na equipe, mas executar e enviar ficam desabilitados com motivo legível. O Orca não deve ser apresentado como mais um modelo ou funcionário quando atua como backend.

## Fora da primeira entrega funcional

Cloud, acesso remoto, multiplayer humano, navegação em primeira pessoa, economia de jogo, importação universal de plugins e criação de um novo runtime de IA. Voz e copiloto de tela estão documentados como fases posteriores, conforme as referências recebidas. O núcleo inicial entrega comunicação textual observável, coordenação, equipes, autonomia e parada.

## Critérios de produto

- Um usuário consegue configurar equipe, enviar objetivo, conversar diretamente, responder perguntas e revisar resultado pelo localhost.
- Toda tarefa iniciada por mensagem direta apresenta registro de ciência/decisão do coordenador anterior ao dispatch.
- Ao clicar em um avatar é possível identificar provider, tarefa, fila, conversa e evidências.
- Um bloqueio ou falha aparece no escritório e no chat com o mesmo significado.
- Histórico recupera conversas e contexto após reinício sem repetir trabalhos já aceitos.
- Nenhum sucesso visual é apresentado como tarefa aprovada sem os gates correspondentes.
- O modo de lista fornece as mesmas ações essenciais do modo 3D.
