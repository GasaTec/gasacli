# 02 — CLI interativa e comandos

## Entrada
`gasa` abre uma TUI de chat na pasta atual. Primeira execução: confiança, descoberta de agentes, escolha inicial de perfis. Sessões e histórico persistem localmente.

## Slash commands planejados
| Comando | Ação |
|---|---|
| /goal | Criar execução supervisionada |
| /agents | Listar, habilitar e configurar agentes |
| /config | Exibir/editar configuração efetiva |
| /prompts | Visualizar e editar templates |
| /skills | Listar e instalar skills com consentimento |
| /brain | Configurar vault Markdown |
| /github | Conectar issues via gh |
| /plugins | Instalar/desativar plugins |
| /status | Mostrar tarefas, workers e gates |
| /resume | Retomar execução |
| /cancel | Cancelar e persistir estado |
| /help | Ajuda |
| /exit | Encerrar chat |

## Comandos fora da TUI
`gasa doctor`, `gasa agents`, `gasa config`, `gasa goal "objetivo"`, `gasa brain`, `gasa github`.

## Local commands

`gasa config [show|set <key> <value>|path|edit]`, `gasa backup [create]`, `gasa backups` and `gasa restore <file> [--preview|--yes]` manage local state. Backups support `config`, `prompts`, `skills`, `agents`, `segundo-cerebro` and `database`, with a versioned manifest and SHA-256 hashes. Add `--encrypt` to create a passphrase-protected archive; the passphrase is requested without echo. Restore requires reviewing `--preview` and passing `--yes` on the CLI. `/prompts` lists and shows existing override files while the prompt subsystem is unavailable.

## Estados de interface
IDLE, TRUST_REQUIRED, PLANNING, EXECUTING, VERIFYING, REVIEWING, NEEDS_APPROVAL, BLOCKED, DONE, CANCELLED. Exibir progresso, agente atual, logs resumidos, consumo real ou estimado devidamente identificado.

## Requisitos
Streaming, teclado, histórico, retomada, cancelamento propagado a subprocessos, modo sem cor, mensagens acessíveis e confirmação de ações destrutivas. Erros devem oferecer próximo passo sem inventar sucesso.

Orchestration, run lifecycle and provider integrations remain planned; local config, backup, restore and prompt file inspection are available.
