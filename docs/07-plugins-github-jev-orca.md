# 07 — Plugins e integrações

## SDK de plugins
Manifesto versionado com nome, versão, origem, permissões, compatibilidade, comandos, hooks e ciclo de vida. Instalação opt-in, revisão de permissões, isolamento e desinstalação. Plugins não podem contornar trust gate nem políticas do core.

## GitHub — primeira integração
Usar `gh` quando disponível, verificar autenticação sem extrair tokens, selecionar repo com consentimento. Ler/listar issues, criar tarefas a partir de /goal após aprovação, vincular runs a issues, comentar progresso e evidências; fechar somente com gates aprovados e política explícita. Sem gh, GasaCLI continua funcional.

O adaptador inicial usa apenas argumentos do `gh` CLI e não lê os arquivos de autenticação. O projeto precisa habilitar a integração em `gasacli/config.yaml` (`plugins.github: true`); `/github enable` ou `gasa github enable` registra esse consentimento. Listagem e leitura recusam projetos sem opt-in. Escritas usam `issue_write`, exibem o comando e o conteúdo para aprovação e aceitam `--dry-run` na CLI. Alterar labels ou responsáveis exige habilitação explícita. A ligação de runs está disponível como API, mas ainda depende do engine fornecer run id e resumo.

Comandos iniciais: `/github [auth|issues [state] [--assignee USER] [--label LABELS] [--search QUERY] [--limit N]|issue <n>|comment <n> <texto>|enable|disable]`; CLI: `gasa github [status|auth|issues [state] [filters]|issue <n>|create --title T --body B|comment <n> <texto>|enable|disable] [--dry-run]`.

TODO no engine: quando `/goal --from-issue <n>` for integrado, obter a issue pelo adaptador e converter com `github.IssueToGoal(issue)` antes de iniciar o planejamento.

## JEV — decisor opcional
`gasa plugin install jev` e `gasa jev connect` são comandos propostos. Conexão interativa e segredo no cofre do SO. JEV sugere modelo/agente por etapa; hard constraints (segurança, orçamento, independência do revisor) permanecem no core. Sem JEV, usar roteamento fixed.

## Orca — duas direções
Outbound: GasaCLI usa Orca como executor opcional.
Inbound: Orca apresenta GasaCLI como agente conversacional selecionável. Isso exige verificar suporte upstream a registro de agentes externos e contrato de sessão/stream/cancelamento; não presumir que já existe. Proposta: `gasa integration install orca`. Prevenir recursão entre runtimes.

## Linear — posterior
Adapter de issue tracker independente do GitHub, sem obrigatoriedade para core.

## Segurança
Permissões granulares: ler/escrever projeto, executar shell, ler/escrever vault, rede, issues e segredos. Sem instalação silenciosa. Eventos e logs devem ocultar dados sensíveis.
