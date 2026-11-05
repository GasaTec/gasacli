# Escritório de agentes no localhost

Data: 2026-11-04. Estado: proposta de evolução solicitada pelo usuário; documentação para implementação, sem alteração do runtime.

A experiência alvo do GasaCLI é um escritório virtual no navegador local. O usuário poderá conversar com o orquestrador ou com qualquer integrante da equipe, acompanhar os agentes trabalhando e ver as mensagens entre eles. Claude Code, Codex, Antigravity, Kimi e outros agentes usarão as instalações e contas da própria máquina, conforme capacidades verificadas. O Orca será o backend preferencial para executar e supervisionar essa equipe.

O terminal continua disponível para iniciar o aplicativo, diagnóstico e automação. As operações diárias devem poder ser realizadas pelo localhost, sem copiar comandos ou abrir um terminal para cada agente.

**Diretriz confirmada pelo usuário:** substituir completamente a aparência de IDE/Orca no localhost. O escritório 3D ocupa a experiência principal; chat, equipe e controles aparecem integrados à cena. Reaproveitar o motor não implica reaproveitar o layout do Orca. Durante esta etapa não executar `gasa`, iniciar localhost ou testar o aplicativo; somente quando o usuário solicitar.

## Ordem de leitura

| Documento | Conteúdo |
| --- | --- |
| [01 — Pesquisa e evidências](01-pesquisa-e-evidencias.md) | O que existe no GasaCLI, Orca e Openbot; fontes e limitações verificadas |
| [02 — Produto e fluxos](02-produto-e-fluxos.md) | Escritório, equipe persistente, canais, chat direto e critérios de produto |
| [03 — Arquitetura e integração](03-arquitetura-e-integracao.md) | Go, localhost, adaptador Orca, plugins, persistência e evolução do código |
| [04 — Comunicação e coordenação](04-comunicacao-e-coordenacao.md) | Protocolo para conversar, registrar intenções, autorizar trabalho e recuperar falhas |
| [05 — Especificação do escritório 3D](05-especificacao-3d.md) | Cena, avatares, estados reais, interação, acessibilidade e orçamento de renderização |
| [06 — Entrega e validação](06-entrega-e-validacao.md) | Fases executáveis, arquivos envolvidos, testes e definição de integração concluída |
| [07 — Requisitos das imagens](07-requisitos-das-imagens.md) | Mapeamento das 12 referências, equipes, autonomia, assinaturas, voz e copiloto |
| [SDDs — Telas do novo sistema](sdd/README.md) | Mapa de telas, criação com bichinho, mesa, descoberta/instalação, principal/fallback e dashboard |
| [ADR-0021](../adr/0021-localhost-office-and-orca-runtime.md) | Decisão proposta e relação com os ADRs existentes |

## Direção recomendada

**GasaCLI como produto e interface; Orca como execução e supervisão; Openbot como referência de organização de equipe.** A cena 3D representa o estado observado. Animações não atribuem tarefas, entregam mensagens ou concluem trabalho.

Três caminhos foram considerados:

| Caminho | Benefício | Custo / limite |
| --- | --- | --- |
| Interface localhost + adaptador Orca, recomendado | Reaproveita o Go existente e as capacidades reais do Orca | Exige descobrir capacidades, conciliar estado e manter testes de contrato |
| Tudo como plugin dentro do Orca | Menor distância da interface desktop do host | API de plugins não equivale à API completa da CLI; não resolve sozinha o localhost independente |
| Novo motor ou fork do Openbot | Controle integral do produto | Duplica execução, sessões e plugins; aumenta manutenção e exige avaliar a licença do código incorporado |

A meta de “100% integrado” significa que todas as funcionalidades acordadas têm caminho suportado e testes reais na versão fixada do Orca. Não significa declarar compatibilidade com toda CLI, plugin, modelo e versão antes de verificá-los. A matriz de aceite está no documento 06.

## Escopo desta entrega documental

- Estudo do código local e de snapshots dos dois repositórios externos.
- Arquitetura, experiência, protocolo, integração, sequência de implementação e critérios de aceite.
- SDDs das telas e contratos de cadastro de agentes, instalação explícita de ferramentas e troca principal/fallback.
- Leitura das duas imagens encontradas na sessão Claude indicada e das 12 imagens enviadas diretamente nesta conversa, sem copiar conversas ou imagens privadas para o Git.
- Nenhuma implementação de 3D, instalação de plugin, lançamento de agente, alteração de banco ou publicação.

## Referência visual disponível

As 12 imagens recebidas diretamente mostram o estúdio publicado por Fernando Nogueira: ambiente 3D com mesas, personagens estilizados, reunião, editor de agentes, equipes e configurações. Elas passam a ser a referência de direção visual. A captura antiga do Orca serve apenas como contexto histórico e não orienta o novo layout.

As imagens mostram estados estáticos e descrevem comportamentos; não comprovam seu funcionamento, tecnologia ou APIs. A proposta implementará comportamento equivalente com identidade própria do GasaCLI, sem copiar marca, personagens ou assets do autor.

## Relação com a documentação anterior

O código atual continua sendo o descrito pelo [README do projeto](../../README.md). Os documentos desta pasta descrevem o destino, não capacidades já entregues.

Preservam-se o servidor loopback e distribuição do [ADR-0009](../adr/0009-localhost-dashboard.md), o contrato de coordenação do [ADR-0015](../adr/0015-session-message-bus.md), as permissões do [ADR-0019](../adr/0019-permission-policy-and-approval-events.md) e os gates de qualidade. O ADR-0021 propõe tornar o Orca preferencial para novos runs do escritório; o motor nativo continua necessário para compatibilidade e uso independente.
