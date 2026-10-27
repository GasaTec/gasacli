# Modelo de segurança e permissões — baseline v1

## Ameaças
Arquivos do projeto e vault podem conter prompt injection. Plugins podem executar código hostil. Processos filhos podem vazar segredos. Localhost pode receber requisições de páginas maliciosas. Logs podem conter dados privados. Sync pode expor conteúdo se mal projetado.

## Trust e autorizações
Confiar por caminho canônico e identidade do projeto; revalidar symlinks e mudanças de diretório. Escopos distintos: read_project, write_project, execute_commands, install_dependencies, network, read_vault, write_vault, manage_issues, manage_plugins, access_secrets. Aprovações explícitas para ações sensíveis, com escopo e expiração. Nunca deixar LLM aprovar a própria ação.

## Execução
Subprocessos sem privilégios adicionais; limites de tempo, concorrência e tamanho de saída; diretório e ambiente controlados; evitar shell string interpolation; passar argumentos por vetor; mascarar credenciais; encerrar árvore de processos ao cancelar quando suportado.

## Segredos
Usar cofre de credenciais do SO quando disponível. Config guarda apenas referências. Nunca registrar tokens, chaves, cookies, envs sensíveis ou credenciais no SQLite, logs, URL ou argumentos de CLI. .env.example sem valores reais.

## Dashboard local
Bind loopback 127.0.0.1. Autenticação local com segredo aleatório, permissões restritas, Host/Origin validation, CSRF, CORS restrito e cookies seguros. Porta 3199 preferida, fallback automático sem matar processo externo. Não expor API remota por padrão. Service worker não armazena dados sensíveis.

## Dados
SQLite e vault locais por padrão. Cloud opt-in; desenho de E2EE com chave controlada pelo usuário, revogação de dispositivos e limitação clara de recuperação. Terceiros (Claude/Codex) podem processar dados enviados a eles conforme seus termos.

## Plugins
Manifesto assinado/verificável quando suportado, origem, versão fixada, permissões visíveis, aprovação antes de instalar, isolamento e revogação. Conteúdo de skill não altera política de segurança.

## Segurança open-source
Canal privado para vulnerabilidades, secret scanning, análise de dependências, CI mínimo e releases com checksums/assinatura. Revisão de titularidade/licenças antes de tornar público.

## Critérios de validação pendentes
Testes adversariais de prompt injection, CSRF, path traversal, symlinks, escalada de permissões, cancelamento e vazamento de segredo. Este documento é desenho de controles, não certificação.
