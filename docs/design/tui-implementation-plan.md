# Plano de implementação da TUI profissional

**Objetivo:** evoluir a interface interativa existente com dados reais, mantendo a CLI tradicional, a política de confiança e as integrações compartilhadas.

**Arquitetura:** `cmd/gasa` monta a sessão; `internal/tui` apresenta comandos e eventos; `internal/engine`, `internal/orchestration`, `internal/adapters`, `internal/sessions` e `internal/permissions` continuam responsáveis por execução, persistência, providers e autorização. A TUI deve receber capacidades e snapshots explícitos, sem executar processos ou acessar dados privados diretamente no renderizador.

**Stack:** Go 1.24, Bubble Tea, Bubbles, Lip Gloss e Glamour; a renderização usa limites de tamanho e evita o processamento de linhas excessivamente longas.

## Diagnóstico e limites

- `Session.Run` oferece modo interativo e textual; o modelo existente tem textarea multilinha, viewport, spinner, paleta/autocomplete, confiança e aprovação, execução assíncrona e cancelamento.
- `executeCommand` e o registro de comandos são comuns aos modos; `/goal` chama a orquestração real, com eventos e métricas disponíveis nos adapters e engine. O armazenamento durável do engine usa SQLite.
- `sessions.ScanMetadata` lê sessões locais de Claude e Codex; `/sessions` filtra pelo projeto confiável, `/use` seleciona uma sessão no modo interativo e texto livre ou `/send` a retomam após verificações de permissão. Isso não inicia uma conversa nova nem equivale a chat genérico com qualquer provider.
- A descoberta identifica mais CLIs do que a orquestração suporta; disponibilidade de uma CLI não comprova uma execução ativa. Custo e tokens só podem ser apresentados quando o provider os reportar.
- `Ctrl+C`, `Ctrl+D`, `Esc`, `Tab`, `Enter`, `Ctrl+S` e as teclas de multiline já têm semântica em `internal/tui/interactive.go`. Mudanças devem preservar os comportamentos contextuais e aprovação.

## Etapas e critérios verificáveis

### 1. Fundação e layout

- [x] Separar o cálculo de dimensões da composição visual mantendo textarea, viewport, trust e handlers existentes.
- [ ] Criar uma paleta de estilos com opções clara e escura e respeitar `NO_COLOR`; validar contraste e fallback sem cor. Configurar tema somente pelo mecanismo de configuração existente, com valor padrão compatível.
- [x] Exibir versão fornecida pela aplicação, projeto e estado observado; ocultar a lateral automaticamente em largura pequena.
- [x] Testar renderização e resize em 120×36, 80×24 e dimensões menores, incluindo diálogos de aprovação.

### 2. Navegação e seleção

- [x] Alimentar a lateral com descoberta real e projetos confiáveis. A execução ativa é indicada no rodapé; a descoberta não é apresentada como execução.
- [x] Adicionar seletor de projeto e paleta, com atalhos configuráveis que preservam rascunhos e respeitam aprovações.
- [x] Alternar entre projetos do registro de confiança, revalidando o caminho, bloqueando execução ativa e preservando rascunhos ao recusar a troca.
- [ ] Selecionar agente apenas entre capacidades efetivamente suportadas e revalidar disponibilidade no momento da execução. Cobrir ausência e falhas de descoberta com fixtures.

### 3. Conversa, histórico e execução

- [ ] Manter `/goal` e comandos pelo dispatcher existente; conectar eventos de `orchestration.Options.OnEvent` e resultados persistidos a mensagens tipadas e limitadas no modelo, sem bloquear o loop Bubble Tea.
- [x] Integrar `/history <id>` ao índice e transcript de `internal/sessions`, restrito ao projeto confiável, com texto e resultados disponíveis.
- [x] Para mensagens livres, usar somente uma sessão selecionada com suporte real a `sessions.Sender`; reaproveitar a aprovação auditada de `/send`, streaming, cancelamento e limites existentes. Sem sessão ou suporte, explicar a indisponibilidade. Validado com fixtures; falta prova com provider autenticado.
- [x] Renderizar Markdown e blocos de código com Glamour, sanitizar controles e limitar o processamento; preservar a posição de leitura durante eventos.
- [x] Exibir tokens e custo do último agente apenas quando informados; medir RSS no Linux e working set no Windows, com fallback de heap Go nos demais sistemas.
- [ ] Testar streams, erro, cancelamento, aprovação negada, projeto não confiável e ausência de métricas com providers falsos; executar `go test ./internal/tui ./internal/sessions ./internal/orchestration ./internal/engine`.

### 4. Qualidade e documentação

- [ ] Atualizar `internal/tui/README.md`, `README.md` e documentação de comandos para descrever atalhos e limites realmente implementados, incluindo seleção de sessão e fornecedores suportados.
- [ ] Executar `go test ./...`, `go vet ./...` e `go build ./...`; executar lint ou verificações adicionais definidas pelo repositório. Registrar falhas preexistentes separadamente.
- [ ] Compilar para Windows, Linux e macOS; testar a interação em terminais reais em cada plataforma antes de afirmar compatibilidade operacional. Medir memória e responsividade com histórico grande, documentando método e limites.
- [ ] Revisar diff e preservar alterações concorrentes; verificar que CLI tradicional, modo sem TTY, trust, permissões, persistência e dashboard não regrediram.

## Aceite

A entrega exige execução interativa real de `gasa`, comandos tradicionais funcionais, projetos e agentes de fontes reais, chat conectado a uma sessão suportada, acompanhamento de execuções, adaptação a resize, métricas sem valores simulados, documentação atualizada e testes/build aprovados. Testes de modelo e compilação cruzada não substituem prova em terminal real nem prova de provider autenticado.
