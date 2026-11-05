# SDD — Telas e comportamento do estúdio GasaCLI

Data: 2026-11-04. Estado: especificação proposta para implementação. SDD significa especificação de desenho do sistema; os contratos deste conjunto são propostas, não APIs ou telas já existentes.

Complementa a [visão do escritório](../README.md), detalhando a solicitação de telas, criação com bichinho, instalação de ferramentas e configuração principal/fallback. O localhost terá identidade própria e escritório 3D como entrada. Não executar GasaCLI, iniciar localhost, instalar ferramentas ou testar o produto até solicitação do usuário.

## Documentos

| Documento | Responsabilidade |
| --- | --- |
| [SDD-01 — Mapa de telas](01-mapa-de-telas.md) | Navegação, rotas conceituais, conteúdo, ações, estados e referência das imagens |
| [SDD-02 — Criar e gerenciar agentes](02-criar-e-gerenciar-agentes.md) | Escolher bichinho, identidade, modelo, equipe e fazer a mesa aparecer |
| [SDD-03 — Ferramentas, instalação e fallback](03-ferramentas-instalacao-e-fallback.md) | Descoberta na máquina, estados, instalação explícita e troca segura de provider |
| [SDD-04 — Dashboard e operação](04-dashboard-e-operacao.md) | Métricas, tarefas, conversas, aprovações, configurações e aceite integrado |
| [SDD-05 — Composição e jornadas](05-composicao-e-jornadas.md) | Desenho das páginas, distribuição visual, navegação e fluxo completo do bichinho até a mesa |

## Decisões de experiência

1. Escritório é a página inicial; Dashboard é uma página própria de dados.
2. Equipe mostra cartões de todos os personagens e permite criar, editar, conversar e arquivar.
3. Escolher bichinho personaliza o personagem; escolher Claude/Codex/Kimi/Antigravity define a ferramenta que ele usa. São decisões independentes.
4. “Criar e colocar no escritório” persiste personagem, vínculo com o escritório e mesa. Não lança CLI nem tarefa.
5. Principal é obrigatório para um agente operacional; fallback é opcional e ordenado. Agente incompleto pode ser salvo como rascunho.
6. Ferramentas encontradas, prontas, ausentes e incompatíveis aparecem com estados distintos. As ausentes oferecem “Instalar” ou orientação oficial conforme suporte real.
7. Disponibilidade da ferramenta é global ao host; personagem, equipe, conversa e mesa pertencem ao contexto explicitamente escolhido.
8. Toda ação de trabalho continua passando pelo coordenador; fallback não amplia ferramentas, escopo, permissão ou autonomia.

## Requisitos rastreáveis

| ID | Requisito | Detalhamento |
| --- | --- | --- |
| UI-01 | Escritório 3D principal, sem layout de IDE/Orca | SDD-01, tela T01 |
| UI-02 | Tela de equipe, cartões e duplas/trios | SDD-01 T02; SDD-02 |
| AG-01 | Bichinho selecionável, cor e acessórios | SDD-02, etapa 1 |
| AG-02 | Nome, papel, quando chamar e instruções | SDD-02, etapas 2 e 4 |
| AG-03 | Principal, modelo, esforço e fallback por agente | SDD-02 etapa 3; SDD-03 |
| AG-04 | Salvar cria personagem e mesa uma única vez | SDD-02, persistência e eventos |
| TOOL-01 | Descobrir ferramentas do próprio host | SDD-03, catálogo e diagnóstico |
| TOOL-02 | Instalar ausentes por ação explícita e verificável | SDD-03, instalação |
| ROUTE-01 | Fallback ordenado sem executar duas tentativas | SDD-03, política de troca |
| CHAT-01 | Chat com time, coordenador ou pessoa | SDD-04, conversas |
| COORD-01 | Comunicar coordenador antes de trabalhar | SDD-04; protocolo do documento 04 |
| DATA-01 | Dashboard por período, agente, ferramenta e modelo | SDD-04, dashboard |
| OPS-01 | Aprovações e parada de tarefa/equipe | SDD-04, operação |
| CFG-01 | Ajustes de ferramentas, autonomia, aparência e integrações | SDD-01 T10–T14; SDD-04 |
| EXT-01 | Voz por pessoa e copiloto de tela em fase posterior | SDD-01 T13/T15; SDD-04 |

## Ordem de construção

Primeiro implementar contratos de dados e ferramentas, depois criação/edição e sincronização de mesa, chat/coordenação e estados da cena. Dashboard deriva do mesmo log. A apresentação pode ser prototipada com fixtures identificadas, mas não conta como execução real. Ver o [plano por fases](../06-entrega-e-validacao.md).
