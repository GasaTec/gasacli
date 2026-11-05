# 05 — Especificação do escritório 3D

## Direção visual confirmada

O localhost será um estúdio 3D que ocupa a tela, inspirado nas imagens do LinkedIn recebidas. Ambiente em corte, câmera elevada, piso e móveis quentes, iluminação suave e personagens pequenos de formas simples. Cada agente possui mesa, computador, telefone e caneca. Há área de reunião, café e quadro de atividade da equipe.

O layout antigo de IDE/Orca deixa de ser a interface principal. Chat, destinatário, notificações e controles surgem como interface própria do estúdio sobre a cena. Diff e logs abrem sob demanda. A marca é GasaCLI: preservar os assets autorizados e tokens legíveis, sem copiar a placa “NOGSolutions”, nomes, mascotes ou assets do post.

As capturas fornecem composição e comportamento desejado. Não permitem identificar a engine usada no vídeo, medir performance ou confirmar animações. O movimento abaixo é uma especificação de implementação.

## Cena e objetos funcionais

| Elemento | Função real | Interação |
| --- | --- | --- |
| Mesa do coordenador | Visão do objetivo, plano, equipe e decisões | Abre conversa com coordenador |
| Mesa do especialista | Identidade e tarefa atual | Seleciona agente e conversa direta |
| Monitor | Resumo sanitizado da ação, arquivo, check ou pergunta atual | Expande atividade e evidências |
| Telefone | Solicitação de aprovação ou resposta humana | Abre a solicitação vinculada, com ação e validade |
| Caneca / área de café | Representação de disponibilidade ociosa conhecida | Mostra status e fila; não dispara chamada ao modelo |
| Mesa de reunião | Planejamento, discussão de escopo ou revisão envolvendo membros | Abre canal e decisões daquela reunião |
| Quadro da equipe | Tarefas, bloqueios e resultado observado | Abre visão de tarefas com filtros |
| Arquivo / estante | Histórico e artefatos do projeto | Abre histórico, respeitando trust |

O telefone só chama enquanto existir uma solicitação pendente. Ao resolver, a sinalização termina em todos os clientes. Áudio depende de gesto/consentimento do usuário, com opção de silenciar e equivalente visual; não depender de autoplay do navegador.

O café é uma metáfora opcional para estado ocioso confirmado. Não simular conversa, atividade produtiva ou pausa do provider. Uma pessoa com execução desconhecida não é mostrada descansando com certeza. A decoração pode ser estática; indicadores de trabalho devem corresponder a eventos.

## Organização espacial

Começar com um ambiente para 8 integrantes, incluindo coordenador, com espaços extras reconfiguráveis. Mesas agrupam equipes fixas; o coordenador permanece identificável. Corredores livres conectam mesas, reunião e café.

Posições são preferências visuais por escritório. Um `deskId` referencia um `agentId`; mover uma mesa não altera diretório, worktree, permissões ou responsável de tarefa. Aumentar equipe reorganiza o ambiente sem recriar sessões.

Coordenadas de cena, como largura inicial de 18 e profundidade de 12 unidades, são parâmetros locais ajustáveis. Mesa e cadeira usam escalas consistentes; avatares não podem ocultar rótulos essenciais. A câmera tem enquadramento geral, foco na pessoa e foco na reunião, com zoom limitado.

## Avatares e personalização

Identidade visual própria com corpos geométricos, variações de olhos, antenas, cores e acessórios. Cada avatar preserva nome, papel e provider em rótulo legível. Forma e cor não determinam provider ou capacidade. Um usuário pode ter duas pessoas usando o mesmo provider com funções diferentes.

A galeria inicial proposta inclui raposa, gato, coruja, capivara e robô, com assets próprios. A [SDD-02](sdd/02-criar-e-gerenciar-agentes.md) define escolha do bichinho, configuração e criação transacional de personagem/mesa. Fallback troca a ferramenta efetiva, preservando aparência e identidade; não cria outro bichinho ou outra mesa.

Editor de integrante inclui prévia, nome, papel, quando chamar, provider instalado, modelo/effort suportados, equipe fixa, ferramentas permitidas e instruções. Voz aparece na fase correspondente. Salvar configuração não inicia processo, login, instalação ou geração de imagem.

Clips visuais previstos: sentado/ocioso, levantar, andar, digitar, reunião, aguardar resposta, sinalizar aprovação, entrega e falha. Procedimento simples com geometrias próprias serve ao primeiro protótipo; modelos GLB só quando houver necessidade e licença identificada.

## Trabalho e presença são estados diferentes

| Estado observado | Apresentação |
| --- | --- |
| Identidade sem sessão ativa | Mesa e avatar em modo inativo, rótulo “Sem sessão” |
| Pronto e ocioso | Disponível, parado ou café opcional |
| Pedido aguardando coordenação | Indicador do pedido e conexão com coordenador; sem digitação de trabalho |
| Planejamento aceito para equipe | Participantes convocados à reunião; mostra assunto e canal |
| Dispatch confirmado em execução | Pessoa na mesa, animação de trabalho e monitor com atividade conhecida |
| Mensagem entre agentes | Balão curto e conexão temporária, vinculados à mensagem persistida |
| Pergunta ao usuário | Telefone/aviso e rótulo “Aguardando resposta” |
| Aprovação necessária | Telefone e ação explícita; distinguir de pergunta informativa |
| Verificação/revisão | Monitor apresenta check/revisão; trabalho ainda não aparece como aprovado |
| Resultado aprovado | Conclusão indicada após gates, com acesso ao artefato |
| Falha ou bloqueio | Símbolo e texto com motivo, sem celebração |
| Conexão/estado desconhecido | “Estado desatualizado”, interrompe simulação de progresso |
| Parada solicitada | “Parando”; só fica “Parado” após confirmação |

Movimento nunca é pré-condição do backend: a execução não espera o avatar chegar à mesa. Se a tarefa terminar durante o trajeto, a cena converge ao estado atual. Não prolongar animação para fingir processamento.

Reunião representa uma etapa e suas mensagens reais, não um LLM adicional conversando sem necessidade. Só participantes relevantes se reúnem. Cada atribuição/subtarefa surge do coordenador e respeita o contrato de comunicação.

## Renderização proposta

Adotar inicialmente uma camada isolada com Three.js no frontend existente, evitando impor React/Solid ou reescrever todo o chat. A [documentação oficial](https://threejs.org/docs/) oferece câmeras, geometrias, animação e raycasting pertinentes a essa camada. Versão e bundler serão fixados na implementação após conferir ambiente e licenças; nenhuma dependência é adicionada por esta documentação.

Separação sugerida:

```text
estado do escritório ← snapshot + eventos do backend
        ↓
projeção visual pura: identidade, postura, destino, rótulos
        ↓
cena 3D: câmera, objetos, animações e seleção
        ↓ clique/teclado
ação de interface: selecionar pessoa, abrir chat, pedir parada
        ↓
serviço HTTP autenticado, com política e coordenação
```

O loop de renderização não chama provider, cria tarefa ou modifica o banco. Eventos novos atualizam o estado de domínio; a projeção deriva destinos e poses. Animações podem ser canceladas e retomadas sem mudar o resultado real.

Chat, formulários, aprovações e textos longos usam DOM. Rótulos sobre a cena usam camada acessível e são filtrados por foco/zoom para evitar sobreposição. Picking seleciona objetos com identidade estável; não transformar o texto renderizado do monitor em código executável.

## Caminhos e animações

Usar pontos de passagem ou grade simples dos corredores e movimento interpolado. Não incluir engine física ou navegação multiplayer no primeiro corte. Reservar assentos visuais na reunião por ID para impedir sobreposição. Interromper trajeto ao mudar de projeto, ocultar cena ou desabilitar movimento.

Balões limitam tamanho e duração; mostram autor, destino e resumo sanitizado. No histórico fica a mensagem completa. Duplicação/replay de eventos não repete telefonema, alerta sonoro ou celebração. Marcar o que é replay após reconexão.

## Orçamento inicial de performance

Metas de projeto a medir, sem alegação de resultado atual:

| Critério | Meta inicial |
| --- | --- |
| Cena de referência | 8 agentes, 8 mesas, uma reunião e chat aberto |
| Fluidez | 30 fps estáveis em hardware de referência documentado; até 60 fps se disponível |
| Entrada/chat | Sem bloqueio de digitação por renderização; medir latência p95 |
| Resolução | Limitar pixel ratio e oferecer qualidade baixa/média/alta |
| Assets 3D iniciais | Até 10 MB comprimidos como orçamento; geometrias compartilhadas quando possível |
| Streaming | Agregar updates de texto; não recriar a cena por token |
| Memória | Comparar uso antes/depois e liberar recursos ao desmontar; medir RAM/GPU do navegador separadas do Go |
| Cena oculta | Suspender renderização contínua e reduzir observação conforme necessidade |

Testar também equipe maior, perda de WebGL/contexto e retorno de aba em segundo plano. Degradação reduz sombras, efeitos e movimento antes de prejudicar chat, aprovações e parada. Nenhuma garantia de fps será publicada sem máquina, navegador, resolução e versão registrados.

## Acessibilidade e falhas

Disponibilizar visão de equipe em lista/cartões com as mesmas ações e estado do 3D. Navegação por teclado, foco visível, nomes acessíveis, modo de movimento reduzido e informação além de cor. Anúncios agrupados evitam ler cada token ou movimento.

Em falha de asset ou GPU, abrir modo de equipe e chat, com opção de tentar a cena novamente. Não mostrar tela vazia nem bloquear uma aprovação. O painel de parada fica fora do canvas e disponível em qualquer vista.

## Aceite visual e funcional

- A tela inicial mostra o escritório e compositor, sem a composição principal de IDE/Orca.
- Cada integrante tem mesa e objetos selecionáveis com função identificada.
- Planejamento/reunião, digitação, mensagens e telefone refletem eventos reais do backend.
- Clique no agente e no monitor abre a conversa/atividade correta do projeto ativo.
- Editor cria identidade visual persistente sem lançar trabalho.
- Reconectar restaura o presente sem repetir historicamente sons e gestos.
- Modo de lista, teclado e movimento reduzido permitem concluir os mesmos fluxos.
- Similaridade de composição é avaliada contra as imagens recebidas; marca e assets permanecem próprios.
