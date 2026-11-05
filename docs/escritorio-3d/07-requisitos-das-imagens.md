# 07 — Requisitos derivados das 12 imagens

## Como interpretar as referências

O usuário enviou 12 capturas do post/vídeo do estúdio de Fernando Nogueira e determinou mudança completa da aparência do localhost. Estas capturas são referências de produto e composição. Não são documentação técnica do aplicativo retratado e não comprovam integração dos recursos com Orca ou GasaCLI.

Não copiar marca, personagens, assets, nomes de agentes, modelos e métricas para a implementação como se fossem dados reais. Os requisitos abaixo traduzem a referência para o GasaCLI.

| Imagem | Conteúdo observado | Requisito de produto | Fase |
| --- | --- | --- | --- |
| 1 | Texto descreve escritório, reunião, telefone, café, copiloto, vozes, equipes, autonomia e parada | Incorporar a visão e separar entregas por capacidade real | Todas |
| 2 | Escritório em corte com mesas, computadores, personagens, reunião, café e sofá | Cena funcional como tela principal | P4 |
| 3 | Elenco de personagens com nome/status | Identidades persistentes e avatar próprio por integrante | P2/P4 |
| 4 | Cartões de agentes com papel, provider, modelo, esforço e equipe | Gerenciar equipe e capacidades instaladas | P2/P3 |
| 5 | Duplas/trios e ações conversar/editar | Equipes fixas e conversa individual | P3 |
| 6 | Editor com corpo, olhos, cor, nome, papel e provider | Criador de integrante com prévia e campos validados | P2/P4 |
| 7 | Editor de equipe, ferramentas e especificação | Papéis, instruções e permissões independentes da aparência | P2/P3 |
| 8 | Copiloto com seleção de tela, objetivo e intervalo | Extensão futura de observação/controle com consentimento | P7 |
| 9 | Dados de assinatura, janela de uso e limite indisponível em um provider | Uso e limites com fonte/horário e ausência explícita | P6 |
| 10 | Seleção e prévia de voz por integrante | Voz opcional por agente, sem cobrança implícita | P7 |
| 11 | Conta de voz externa e fallback citado para voz do Windows | Separar TTS local de serviço externo; credenciais fora da UI | P7 |
| 12 | Tarefas, tempo, arquivos e agregações por agente/modelo/dia | Analytics a partir de eventos deduplicados | P6 |

Os números P0–P7 referem-se ao [plano de entrega](06-entrega-e-validacao.md).

O detalhamento das páginas e painéis está nas [SDDs](sdd/README.md). O requisito adicional do usuário especifica bichinho selecionável, aparecimento da mesa ao criar, lista de IAs detectadas na máquina, botão de instalar ausentes e configuração de principal/fallback por integrante.

## Conta e assinaturas existentes

A visão desejada usa os CLIs e assinaturas que o usuário já possui, sem exigir uma nova API paga por consumo para o núcleo de programação e coordenação. Cada capacidade precisa funcionar pelo caminho oficialmente suportado pelo provider/Orca e pela conta selecionada.

Não usar fallback silencioso para chave API, não prometer gratuidade universal e não assumir que toda modalidade de geração de imagem/vídeo está incluída numa assinatura de chat/código. Mostrar indisponibilidade quando o recurso não estiver acessível por integração autorizada.

Uso precisa separar tokens informados, contagem de tarefas, tempo de parede, quota da assinatura e eventual estimativa equivalente de API. Estimativa não é cobrança. Limite não informado aparece como “Não informado pelo provider”, nunca 0%, ilimitado ou cálculo inventado a partir de tokens.

As telas de referência mostram nomes e níveis de modelos; a implementação descobre opções reais da instalação. O catálogo não deve copiar os nomes exibidos nas imagens como configurações garantidas.

## Equipes e especializações

Permitir dupla de frontend/backend e trio de mídia com direção de arte, imagens e vídeo, como configurações iniciais editáveis. São papéis de equipe, não prova de que o provider selecionado dispõe de ferramentas de mídia. A execução depende das capacidades homologadas e dos plugins já disponíveis no host.

Ao chamar um integrante, o coordenador identifica quais parceiros precisam trabalhar, distribui subtarefas e registra sua decisão. Grupos não significam convocar todos os integrantes a cada mensagem.

## Autonomia e parada

Os três níveis descritos na referência viram rótulos de produto sobre políticas verificáveis: perguntar tudo; executar decisões do plano aceito; trabalhar no escopo permitido, parando antes de ações protegidas. A regra de comunicar o coordenador continua em todos os níveis.

Parada global precisa bloquear novos lançamentos e solicitar interrupção dos workers corretos, preservando estado e arquivos. Não prometer “parada instantânea” universal: a UI mostra pendências e resultado desconhecido quando o host não confirma.

## Voz por agente: extensão planejada

Preferência de voz por identidade, controle de volume, prévia voluntária, fila de fala e silêncio global. Falar somente mensagens públicas selecionadas; não vocalizar secrets, raciocínio oculto ou logs completos. Texto permanece disponível sem áudio.

Preferir voz local comprovada quando a restrição for ausência de custo por uso. Não presumir que todas as vozes de navegador são offline. Serviço externo de voz, como o citado na referência, é uma integração opcional e explicitamente configurada; sua presença na imagem não autoriza contratação, uso de créditos ou envio de conversas.

A CSP e `Permissions-Policy` atuais desabilitam microfone. Síntese de texto e captação de fala têm requisitos distintos: só rever permissões para um recurso implementado e consentido, sem relaxar a política global preventivamente.

## Copiloto de tela: extensão planejada

Separar observação de tela e controle de mouse/teclado. Usuário escolhe o que compartilhar, objetivo, duração e permissão. O coordenador acompanha a intenção e o backend registra o grant. Não capturar tela nem controlar desktop como consequência de criar um agente ou entrar no escritório.

A referência menciona F12 para retomar controle e expiração em dez minutos. Isso vira requisito de avaliação, não garantia da página web: o navegador sozinho não fornece hotkey global confiável nem controle universal do desktop. Verificar as capacidades públicas do Orca/computer-use e eventual helper nativo, incluindo revogação fora da página, timeout no backend e parada em perda de conexão.

Sem caminho suportado, manter o controle indisponível. Não desenvolver por inferência um driver de automação nem capturar dados em segundo plano. Overlay acima de outras aplicações também depende de integração desktop e deve ser uma subentrega separada.

## Comportamentos a demonstrar depois

As fases funcionais devem fornecer demonstração com dados sintéticos, quando o usuário autorizar testes: pedido ao time, reunião dos participantes, dispatch, mensagem entre agentes, pergunta, telefone de aprovação, resolução, revisão e parada global. A demonstração precisa correlacionar cada mudança visual ao evento que a originou.

Uma animação com fixtures demonstra somente a apresentação. Uma tarefa real demonstra somente o backend/agent/host testados. Só a combinação comprovada justifica chamar o escritório de funcional.
