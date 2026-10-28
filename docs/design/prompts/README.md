# Prompts de design do GasaCLI

Índice com uma prompt por tela da experiência completa de TUI e Web. `exists`, `partial` e `planned` descrevem o código observado em 28/10/2026; planned é proposta visual, não recurso pronto.

## Como usar

1. Leia [`../DESIGN.md`](../DESIGN.md) e escolha [`00-design-system-tui.md`](00-design-system-tui.md) ou [`00-design-system-web.md`](00-design-system-web.md).
2. Copie o bloco final `PROMPT` da tela para o gerador e inclua o design system da superfície antes do prompt individual.
3. Para TUI, gere nos tamanhos exatos 80x24 e 120x36. Para Web, preserve três painéis no desktop e inclua os estados responsivos.
4. Use a coluna Status para separar o que existe, está parcial e continua planejado. Dados são fictícios e métricas indicam proveniência.

Cada arquivo descreve propósito, objetivo do usuário, layout, wireframe, estados, dados, interações, acessibilidade e escopo excluído. Instruções para geração em inglês; textos de interface em pt-BR.

## TUI

| Tela | Superfície | Issues | Status |
|---|---|---|---|
| [Welcome and splash](tui-01-welcome.md) | TUI | #1,#22,#31 | planned |
| [Onboarding language](tui-02-onboarding-language.md) | TUI | #31 | planned |
| [Onboarding detected agents](tui-03-onboarding-detected-agents.md) | TUI | #5,#31 | partial |
| [Onboarding install tools](tui-04-onboarding-install-tools.md) | TUI | #31 | partial |
| [Onboarding trust folder](tui-05-onboarding-trust-folder.md) | TUI | #6,#31 | partial |
| [Onboarding permission profile](tui-06-onboarding-permission-profile.md) | TUI | #25,#31 | planned |
| [Onboarding dashboard and Community/Pro](tui-07-onboarding-dashboard-community-pro.md) | TUI | #31,#33,#32,#19 | planned |
| [Onboarding summary](tui-08-onboarding-summary.md) | TUI | #31,#33 | partial |
| [Main chat](tui-09-main-chat.md) | TUI | #1,#11,#21,#22 | partial |
| [Command palette and slash autocomplete](tui-10-command-palette.md) | TUI | #11,#22 | partial |
| [Help command](tui-11-help.md) | TUI | #11,#22 | exists |
| [Agents and doctor](tui-12-agents-doctor.md) | TUI | #5,#9,#21 | partial |
| [Status](tui-13-status.md) | TUI | #7,#21 | partial |
| [Goal Run and task DAG](tui-14-goal-run-dag.md) | TUI | #3,#4,#7,#10,#26,#29 | planned |
| [Approval dialog](tui-15-approval-dialog.md) | TUI | #19,#25 | planned |
| [Permissions effective policy](tui-16-permissions.md) | TUI | #25 | planned |
| [Review and quality gate result](tui-17-review-gate-result.md) | TUI | #10,#21 | planned |
| [Run history and resume](tui-18-history-resume.md) | TUI | #7,#10,#29 | planned |
| [Second-brain search](tui-19-brain-search.md) | TUI | #2 | planned |
| [Skills registry and consent](tui-20-skills-registry.md) | TUI | #8,#12 | planned |
| [Backup and restore](tui-21-backup-restore.md) | TUI | #28,#30 | planned |
| [Web dashboard control](tui-22-web-control.md) | TUI | #22,#33 | partial |
| [GitHub issue picker](tui-23-github-issue-picker.md) | TUI | #13 | planned |
| [Observability monitor](tui-24-observability-monitor.md) | TUI | #21,#24 | planned |
| [Errors and empty states](tui-25-errors-empty-states.md) | TUI | #5,#11,#22 | partial |
| [Narrow and wide terminal variants](tui-26-terminal-size-variants.md) | TUI | #11,#21 | partial |

## Web

| Tela | Superfície | Issues | Status |
|---|---|---|---|
| [First-run connect/login by single-use code](web-01-first-run-connect-code.md) | Web | #19,#32 | planned |
| [Workspace shell: sidebar and tabs](web-02-workspace-shell.md) | Web | #22,#23 | partial |
| [Project overview](web-03-project-overview.md) | Web | #23,#24,#27 | partial |
| [Session chat view](web-04-session-chat.md) | Web | #21,#22 | partial |
| [Multi-agent Run and DAG](web-05-multi-agent-run-dag.md) | Web | #7,#10,#26,#29 | planned |
| [Agents and roles](web-06-agents-roles.md) | Web | #5,#9,#21 | planned |
| [Messages bus viewer](web-07-messages-bus.md) | Web | #21,#29 | planned |
| [Global analytics](web-08-global-analytics.md) | Web | #24 | planned |
| [Per-project analytics drilldown](web-09-project-analytics.md) | Web | #24,#23 | planned |
| [Monitor RAM/CPU history](web-10-monitor.md) | Web | #20,#21 | planned |
| [Logs](web-11-logs.md) | Web | #21 | planned |
| [Permissions and approvals inbox](web-12-permissions-approvals.md) | Web | #25,#19 | planned |
| [Prompts editor](web-13-prompts-editor.md) | Web | #12 | planned |
| [Skills registry](web-14-skills.md) | Web | #8,#12 | planned |
| [Plugins GitHub](web-15-plugins-github.md) | Web | #13 | planned |
| [Brain and memory browser](web-16-brain-memory.md) | Web | #2 | planned |
| [Backup and restore](web-17-backup-restore.md) | Web | #28,#30 | planned |
| [Settings: launch, theme, language, directories](web-18-settings.md) | Web | #31,#33 | planned |
| [Cloud account and Pro activation](web-19-cloud-pro-activation.md) | Web | #19,#32 | planned |
| [Sync status](web-20-sync-status.md) | Web | #15,#19 | planned |
| [Empty, loading, error and offline states](web-21-empty-loading-error.md) | Web | #22,#23 | partial |
| [Mobile and narrow web layout](web-22-mobile-narrow.md) | Web | #22,#23 | planned |

## Issues sem tela de produto

Issues #14–#18 tratam de licença, estrutura/tooling, CI e governança de lançamento; não definem telas de produto. Por isso não há prompt visual dedicado para elas.
