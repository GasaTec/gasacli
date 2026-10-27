# Especificação funcional — baseline v1

Status: especificação aprovada para implementação, não validada por software.

## Produto
GasaCLI é um orquestrador local-first MIT. `gasa` abre TUI e navegador padrão no dashboard local. `/goal` inicia execução durável; conversa normal não altera arquivos automaticamente.

## Requisitos verificáveis
R01: Solicitar confiança por diretório canônico antes de acessar o projeto; guardar decisão e permitir revogar.
R02: Descobrir Claude Code e Codex instalados, informar versão e readiness sem ler tokens.
R03: Configuração fixa YAML e templates editáveis de orquestrador, developer, TL e tester.
R04: Planejar objetivo com critérios de aceite observáveis e tarefas com dependências.
R05: Persistir runs, dispatches, eventos e checks em SQLite com migrações.
R06: Executar agente real e capturar stdout/stderr com streaming limitado.
R07: Verificar testes/build quando aplicáveis; não declarar sucesso sem evidência.
R08: Exigir revisor independente conforme política; bloqueio se indisponível.
R09: Limitar retries, tempo, concorrência e orçamento; cancelar e retomar.
R10: Expor dashboard local preferencialmente 127.0.0.1:3199, com fallback seguro.
R11: Exibir agentes, tarefas, eventos, CPU/RAM, tokens e equivalente de API, com origem dos dados.
R12: `/help` e `gasa --help` apresentam comandos e exemplos.
R13: Estado global e por projeto; múltiplos projetos na sidebar são pós-alpha.
R14: JEV, brain, GitHub, Orca e Cloud não são dependências do core.
R15: Nenhuma ação de escrita destrutiva ou plugin de terceiros sem aprovação.
R16: Distribuir Community e código Cloud no mesmo monorepo MIT.

## Critérios de aceite do produto
A primeira alpha é demonstrável com uma pasta confiável, duas CLIs instaladas, um /goal real, testes verificáveis, review cruzado, retomada e dashboard. Quando não houver dados de tokens, exibir N/D. Quando a porta 3199 estiver ocupada por terceiro, selecionar porta livre sem interferir. Sem assinatura adicional obrigatória.

## Fora do MVP
Sincronização Cloud, cobrança, multi-project workspace completo, JEV dinâmico, Orca host, Rust e Linear.

## Evidências futuras
Cada requisito deve ter teste, execução reproduzível e issue associada antes de ser marcado implementado.
