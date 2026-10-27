# 01 — Como funciona o GasaCLI

GasaCLI é um orquestrador open-source e local-first para agentes de programação já instalados e autenticados. O usuário executa `gasa` dentro de um projeto confiável e conversa com uma TUI. `/goal <objetivo>` inicia uma execução supervisionada.

## Jornada
1. Solicitar confiança na pasta antes de ler arquivos.
2. Detectar CLIs, versões e capacidades; diferenciar instalado de autenticado.
3. Carregar config global e sobrescritas do projeto.
4. Interpretar objetivo, stack, critérios de aceite e riscos.
5. Recuperar contexto mínimo do repositório e, opcionalmente, do segundo cérebro.
6. Escolher orquestrador, implementador e revisor por configuração fixa ou JEV.
7. Planejar tarefas e dependências; despachar agentes em worktrees quando necessário.
8. Executar testes, lint, build e revisão independente do diff.
9. Corrigir com tentativas limitadas; concluir DONE apenas com evidências, senão BLOCKED.

## Experiência
```sh
cd meu-projeto
gasa
# /goal Crie uma tela de login
# /status
# /resume
```

Chat comum não deve disparar mudanças automaticamente. GasaCLI não exige conta Gasatec, Orca, JEV, Obsidian, GitHub ou API key adicional. As CLIs externas podem exigir rede e assinatura próprias.

## Definição de pronto
Critérios aceitos e verificados, testes aplicáveis executados, build quando pertinente, revisão independente exigida aprovada para o diff atual, resumo de arquivos e evidências. Nunca declarar DONE baseado apenas na palavra do agente.

Status: especificação, não implementação.
