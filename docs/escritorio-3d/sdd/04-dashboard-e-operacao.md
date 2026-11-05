# SDD-04 — Dashboard, conversas e operação do escritório

## T09 — Dashboard de dados

Página própria acessada pelo destino **Dashboard**. Escritório continua como tela inicial. Cabeçalho com projeto/escritório, período de 7/14/30 dias ou intervalo personalizado, filtros por integrante, ferramenta e modelo, horário de atualização e botão de voltar ao escritório.

Usar gráficos e tabelas legíveis com identidade GasaCLI; as imagens 9 e 12 orientam a organização dos dados. Nunca preencher números demonstrativos como se fossem o uso real.

### Blocos da página

| Bloco | Conteúdo | Ação |
| --- | --- | --- |
| Assinaturas e disponibilidade | Cards por ferramenta, estado, janela/limite/reset reportado, atualização e host | Abrir ferramenta em Ajustes |
| Resumo do trabalho | Tarefas, pedidos do usuário, concluídas verificadas, bloqueadas, canceladas, arquivos e duração | Filtrar Tarefas |
| Tarefas por dia | Contagens por resultado e data, com tabela alternativa | Abrir tarefas do intervalo |
| Por integrante | Nome/bichinho, tarefas, resultado, tempo, principal e ferramenta efetiva | Abrir perfil/conversa |
| Por ferramenta/modelo | Uso informado, tentativas, duração e disponibilidade | Ver execuções relacionadas |
| Saúde da operação | Falhas de infraestrutura, trocas de fallback, confirmações pendentes e reconciliações | Abrir incidente/tentativa |
| Atividade recente | Linha do tempo de atribuições, mensagens, aprovações e entregas | Abrir tarefa ou conversa |

Exibir somente agregados autorizados para o contexto selecionado. Visão geral entre projetos é seleção explícita; não misturar dados de outros projetos ao trocar de tela.

### Definições de métricas

| Métrica | Definição |
| --- | --- |
| Pedido do usuário | Intenção raiz única; mensagens de progresso e retries não aumentam o total |
| Tarefa | Unidade de trabalho única; Dispatch/tentativa é contagem separada |
| Concluída | Gates exigidos aprovados e conclusão do produto registrada |
| Falha de tentativa | Resultado técnico de uma execução; não equivale necessariamente a falha final da tarefa |
| Uso por IA | Provider efetivo da tentativa, mesmo se era fallback |
| Uso por agente | Identidade persistente, acumulando tentativas de seus providers sem duplicar eventos |
| Tempo decorrido | Intervalo de parede da tarefa; tempo de tentativas concorrentes fica separado e nomeado |
| Arquivos | Caminhos únicos identificados por projeto/tarefa; contar alterações por tentativa separadamente se necessário |
| Taxa de sucesso | Concluídas verificadas / tarefas encerradas no recorte; exibir denominador e cancelamentos separados |
| Tokens | Valores informados com fonte, incluindo ausência; não inferir quota de assinatura a partir deles |

Enquanto não houver tarefas encerradas, taxa é “Sem amostra”. Quota/limite ausente aparece como “Não informado”, nunca como zero ou ilimitado. Tempo de tarefas paralelas não é somado e rotulado como tempo de parede.

Uso informado pode ter abrangência da conta/host e incluir trabalho feito fora do escritório. Identificar essa abrangência e separar de métricas internas do projeto. Não extrair credenciais ou inventar rate limits para completar os cards.

Eventos têm origem e chave de deduplicação; replay não incrementa contagem. Filtros de data usam timezone visível; persistência mantém UTC. Custo equivalente estimado, se existir, é seção opt-in claramente separada de consumo de assinatura e cobrança real.

## T06/O01 — Chat com time e integrantes

Compositor do escritório oferece destinatário **Time**, **Coordenador**, pessoa ou canal selecionado. “Time” encaminha ao coordenador para distribuição, não envia a todos automaticamente. “Conversar” no cartão e clicar no bichinho abrem a mesma conversa direta.

Cabeçalho: nome/bichinho, papel, disponibilidade, ferramenta efetiva quando houver tarefa e acesso ao perfil. Principal/fallback salvos ficam nos detalhes; o usuário precisa perceber quando a execução mudou de ferramenta.

Linha do tempo distingue mensagem humana, fala pública do agente, evento do sistema e ação de ferramenta. Linhas de ferramenta ficam recolhidas com nome, alvo e resultado. Não renderizar dumps técnicos ou raciocínio oculto como resposta de conversa.

Mensagem passa por persistida → aguardando coordenação → fila → aceita → resposta/resultado, conforme evidências. UI não chama “lida” uma mensagem apenas aceita pelo host. Na ausência de canal em tempo real do provider, indicar entrega na próxima fronteira de turno.

Anexo tem prévia, remover, tamanho/estado e validação. Se a ferramenta efetiva não suportar imagem/anexo necessário, manter bloqueio e oferecer alternativa explicitamente, sem descartar conteúdo silenciosamente.

## T07/T08 — Tarefas e detalhes

Quadro/lista com fila, coordenação, execução, bloqueios, revisão e encerradas. As colunas são projeções de estados existentes, não novos status inconsistentes no engine. Mostrar responsável e ferramenta efetiva em cada linha.

Detalhe contém pedido, escopo, plano, decisão do coordenador, aprovações, membros/equipe, dependências, tentativa atual e anteriores, principal/fallback, mensagens e evidência. “Ver mudanças” e “Ver checks” abrem painéis sob demanda; o escritório não vira uma IDE permanente.

Nova configuração de agente não reescreve a configuração histórica de uma tentativa. Evidências de revisão identificam artefato/commit e validade. Worker que termina não aparece automaticamente como tarefa aprovada.

## O02 — Aprovações e telefone

Telefone, notificação e detalhe de tarefa apontam para o mesmo pedido com ID/revisão. Mostrar quem solicita, ação, alvo, consequência, validade e aprovar/negar. Se outro cliente respondeu, atualizar todos e desabilitar a decisão antiga.

Pergunta informativa, autorização de trabalho e instalação são tipos distintos. Negar mantém motivo/estado e comunica o coordenador. Não apresentar checkbox “aprovar tudo” como atalho que altera a política global sem revisão.

## O06 — Parar equipe

Botão permanece disponível fora do canvas, nas páginas de operação. Escopo visível é equipe/run/projeto selecionado, não todos os processos do computador. Bloquear novos dispatches e exibir lista com parada solicitada, confirmada, falha ou desconhecida.

Preservar alterações e histórico. “Retomar” exige conciliar estado; não reenvia todos os pedidos. Ferramentas de instalação têm ciclo próprio e não são mortas por engano como se fossem workers de tarefa.

## T05 — Equipes fixas

Listar duplas/trios, objetivo, participantes, papéis e líder. Criar grupo com nome, propósito e seleção de integrantes existentes, permitindo “Criar agente” em fluxo de retorno preservado. Não limitar a equipe inteira do escritório a três pessoas; dupla/trio é um agrupamento.

Remover membro atualiza futuras convocações e exige tratar dependências das tarefas correntes. Arquivar grupo não arquiva automaticamente seus integrantes. Grupos circulares ou repetição de convocações são detectados no coordenador.

## Ajustes

| Tela | Configurações |
| --- | --- |
| Ferramentas T10 | Catálogo, detecção, instalar, login e detalhes; contrato em SDD-03 |
| Orquestração T11 | Coordenador ativo, principal/fallback dele, autonomia, limites e política padrão para novos agentes |
| Integrações T12 | Plugins/skills realmente reportados pelo host, origem, consentimento, capacidade e links para painéis exclusivos do Orca |
| Voz T13 | Voz por personagem, prévia voluntária, silêncio e fonte; fase posterior |
| Aparência T14 | Qualidade 3D, movimento reduzido, áudio de avisos, idioma, câmera e reorganização de mesas |

Som da notificação de telefone e síntese de voz são controles separados. Desativar áudio não oculta aprovações. Reset de layout altera posições visuais, preservando pessoas e trabalho. Defaults globais e overrides por agente mostram claramente sua origem.

Copiloto T15 fica em fase posterior e desabilitado com explicação enquanto não suportado. Não solicitar câmera/microfone/compartilhamento de tela ao entrar no escritório ou criar um bichinho.

## Contratos complementares propostos

| Serviço | Responsabilidade |
| --- | --- |
| Consultar/salvar agente com revisão | Campos do SDD-02; conflitos explícitos e criação idempotente |
| Gerenciar vínculos/equipes/layout | Vínculo com escritório, alocação e atualização visual sem executar agentes |
| Consultar ferramentas/install jobs | Estados e contratos do SDD-03 |
| Consultar dashboard | Agregados derivados de eventos confirmados com filtros, fonte e atualização |
| Observar escritório | Snapshot e stream consistentes, com cursor e replay |

Esses serviços reaproveitam autenticação, trust, política e persistência locais. Não criar um banco de analytics paralelo ou uma segunda fonte de estado no navegador. A API de decisões do coordenador permanece separada de ações comuns do usuário.

## Cenários integrados de aceite

| ID | Cenário | Resultado esperado |
| --- | --- | --- |
| E2E-01 | Escritório sem pessoas → escolher capivara → preencher papel e IA principal → criar | Uma identidade, um cartão e uma mesa; nenhum worker iniciado |
| E2E-02 | Wizard com principal ausente → instalar/conectar pelo fluxo explícito → voltar | Dados preservados; seleção liberada somente após verificação real |
| E2E-03 | Principal indisponível antes de iniciar → fallback autorizado e pronto | Mesmo bichinho/mesa; nova tentativa vinculada; ferramenta efetiva visível |
| E2E-04 | Resposta do principal perdida após possível aceitação | Reconciliação/bloqueio; nenhum fallback concorrente |
| E2E-05 | Pedido direto ao especialista | Coordenador ciente/decisão registrada antes do dispatch |
| E2E-06 | Solicitação de aprovação | Telefone e central abrem o mesmo ID; resolver elimina a sinalização |
| E2E-07 | Replay/reload durante criação ou instalação | Sem duplicar pessoa, mesa ou job; retoma observação |
| E2E-08 | Tarefa conclui usando fallback | Dashboard conta uma tarefa e suas tentativas/uso efetivo separadamente |
| E2E-09 | Parar equipe com um worker desconhecido | Para novas atribuições; resultado agregado parcial/desconhecido |
| E2E-10 | Sem WebGL, teclado ou mobile | Criar agente, conversar, aprovar e parar continuam disponíveis |
| E2E-11 | Editar principal durante trabalho | Nova revisão para próximas tarefas; tentativa corrente mantém evidência histórica |
| E2E-12 | Quota do provider não informada | Dashboard mostra ausência, sem barra enganosa de 0% |

Todos esses cenários são planejados. Execução do GasaCLI, localhost, instaladores e testes funcionais aguarda solicitação do usuário; esta entrega só especifica e revisa documentos.
