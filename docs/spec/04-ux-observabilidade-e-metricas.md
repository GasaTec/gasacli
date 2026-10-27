# UX e observabilidade — baseline v1

## Abertura
`gasa` abre TUI e navegador padrão automaticamente, exceto headless/--no-browser. `gasa ui` abre dashboard existente. Uma instância de engine por máquina, múltiplos clientes. Porta padrão 3199, fallback autenticado.

## TUI
Chat normal, /goal, /help, /agents, /config, /prompts, /skills, /brain, /plugins, /github, /status, /resume, /cancel, /exit. /help agrupa comandos, exemplos e atalhos. Confirmações explícitas em operações sensíveis. Exibir qual agente executa qual papel e qual tarefa.

## Dashboard
Overview global; projetos; run detail; Agents; Messages; Monitor; Logs; Settings. Sidebar de projetos multiativos planejada para pós-alpha. Global e projeto compartilham filtros por período, modelo, provedor, agente, status e execução.

## Métricas
CPU e RSS do processo GasaCLI e da árvore de agentes separadamente; tempo por tarefa; contagem de tentativas; checks; reviews; tokens entrada/saída/cache reportados; custo equivalente de API por modelo e período; métricas de conclusão. Dados com provenance: measured, provider-reported, estimated, unavailable. Não estimar custo de assinatura como cobrança real.

## Pricing engine
Tabela de preços versionada por modelo, modalidade, moeda, data de vigência e fonte. Valores em micros inteiros/decimal exato. Custos somente quando uso e tarifa são conhecidos; caso contrário N/D ou estimativa explicitamente rotulada. Cache tokens tratados segundo tarifa aplicável. Comparações de economia somente com baseline comparável.

## Comunicação entre agentes
Mostrar eventos operacionais, origem, destino, timestamp, objetivo, pedido de revisão e resposta; expandir payload sanitizado. Não prometer acesso ao raciocínio privado dos modelos.

## Performance
Renderização orientada a eventos, SSE com backpressure, lazy-load de painéis, logs rotativos e sampling configurável. Meta preliminar do processo Go: binário <=25MB, RAM idle <=40MB, warm start <=300ms e CPU idle ~0%, não medidos. Browser e subprocessos medidos separadamente.

## Critérios de aceite pendentes
Testar UX com cinco projetos, fallback de porta, consistência global vs soma de projetos, origem dos dados e funcionamento sem token usage reportado.
