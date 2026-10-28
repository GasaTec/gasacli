# CLI e comandos

## O que funciona hoje

`gasa` abre o terminal local. Após a confirmação de confiança para a pasta, a TUI oferece `/help`, `/agents`, `/doctor`, `/setup`, `/status`, `/config`, `/prompts`, `/skills`, `/brain`, `/github`, `/permissions`, `/backup`, `/backups`, `/restore`, `/web`, `/goal`, `/runs`, `/resume` e `/cancel`. Comandos de agentes e provedores dependem das ferramentas instaladas e da autenticação configurada no próprio provedor. Texto livre não é enviado a um modelo pelo GasaCLI.

Comandos locais disponíveis incluem:

| Comando | Comportamento atual |
|---|---|
| `gasa doctor [--json]` | Descobre CLIs locais e informa instalação e prontidão observável. |
| `gasa setup` | Reabre onboarding e pode executar instalações selecionadas após exibir os comandos e obter confirmação. |
| `gasa config show|set|path|edit` | Consulta ou altera a configuração local. |
| `gasa prompts list` | Lista os papéis, a origem efetiva (`default`, `user` ou `project`) e o caminho. |
| `gasa prompts show|path|reset [role]` | Inspeciona ou restaura o prompt local de um papel. |
| `gasa skills` | Lista skills encontradas localmente; instalação e remoção dependem das opções e permissões disponíveis. |
| `gasa brain status|connect|reindex|search|disconnect` | Gerencia o índice local de um vault Markdown conectado. |
| `gasa permissions` | Consulta a política de permissões local. |
| `gasa run`, `runs`, `resume` | Inicia, lista e retoma execuções locais, conforme o estado do orquestrador. |
| `gasa backup`, `backups`, `restore` | Cria, lista e restaura dados locais com prévia. |
| `gasa web open` | Inicia o painel local e aguarda Ctrl+C. |
| `gasa reset`, `gasa uninstall` | Mostra um plano antes de remover dados locais. |

## Limites atuais

O chat não encaminha texto livre a um modelo. O orquestrador local pode executar tarefas com agentes instalados, mas não oferece sincronização em nuvem. Integrações com GitHub usam o CLI `gh` e exigem configuração e escopos adequados. O painel é local; serviços remotos, Linear e recursos Cloud continuam planejados. A lista de comandos registra o estado exposto pelo build atual, que pode variar entre versões.

Comandos de projeto e agente respeitam o estado de confiança e as permissões configuradas. Operações externas ou sensíveis podem solicitar aprovação. A ausência de uma integração ou credencial não é apresentada como execução bem-sucedida.
