# Modelo de segurança e permissões — baseline v1

> Este documento especifica controles planejados; não certifica que estejam implementados. A política canônica de permissões está em [ADR-0019](../adr/0019-permission-policy-and-approval-events.md).

## Ameaças e limites

Arquivos do projeto, issues e vault podem conter prompt injection. Plugins podem executar código hostil. Processos filhos podem vazar segredos. Um site pode tentar acessar o dashboard local. Logs e sync podem expor dados privados se mal projetados. Definir fronteiras e aprovações explícitas; texto de modelo ou conteúdo do projeto nunca altera política.

## Vocabulário canônico

Chaves de política são identificadores estáveis em inglês; traduções pertencem à interface.

| Chave | Escopo |
|---|---|
| `read_project` | Ler arquivos dentro da pasta confiável. |
| `write_project` | Criar, editar ou remover arquivos do projeto. |
| `run_tests` | Executar checks configurados de teste, lint, typecheck e build. |
| `execute_commands` | Executar outros comandos e processos filhos locais. |
| `install_dependencies` | Instalar ou atualizar dependências. |
| `network_access` | Fazer requisições de rede ou permitir rede a processos filhos. |
| `git_mutations` | Alterar estado Git, inclusive checkout, commit, reset, push e branches. |
| `read_vault` | Ler conteúdo explicitamente selecionado do segundo cérebro/vault. |
| `write_vault` | Criar, editar ou remover conteúdo explicitamente selecionado do vault. |
| `issue_write` | Criar, editar ou fechar issues e registros configurados em trackers. |
| `plugin_install` | Instalar ou atualizar plugins, skills ou extensões executáveis. |
| `secret_access` | Ler ou usar credenciais por meio de provedor de credenciais aprovado do SO. |

Os nomes antigos `network`, `manage_issues`, `manage_plugins` e `access_secrets` são aliases de migração para `network_access`, `issue_write`, `plugin_install` e `secret_access`. Não criar permissões paralelas com os nomes antigos. Chaves desconhecidas falham fechadas. Leitura de issues não é grant separado nesta baseline. Confirme no [ADR-0019](../adr/0019-permission-policy-and-approval-events.md) a matriz de perfis, regras de precedência, formato de aprovação e UX de `/permissions` e `/permissoes`.

## Trust e avaliação

Confiar em projeto usa caminho canônico e identidade; revalidar symlinks e mudanças de diretório. Perfil inicial: `restricted`, `balanced` ou `autonomous`; cada permissão recebe `allow`, `ask` ou `deny`. Precedência: restrições de segurança inegociáveis; regra explícita do usuário; restrições de projeto; restrições do agente; default do perfil. Um escopo mais específico pode restringir, nunca elevar uma permissão negada ou pendente no escopo superior. Em conflito, `deny` vence `ask`, e `ask` vence `allow`. Agente não altera a própria política.

Operações destrutivas, divulgação de credenciais e ações fora do limite confiável exigem aprovação específica conforme as restrições de segurança, qualquer que seja o perfil. Aprovação não altera política e vale só para operação, recurso e prazo mostrados. Confiar numa pasta não autoriza todos os comandos.

## Execução e segredos

Subprocessos sem privilégios adicionais; limites de tempo, concorrência e tamanho de saída; diretório e ambiente controlados; evitar interpolação em shell e passar argumentos por vetor; mascarar credenciais; encerrar árvore de processos ao cancelar quando suportado.

Usar cofre de credenciais do SO quando disponível. Configuração guarda somente referências. Nunca registrar tokens, chaves, cookies, ambientes sensíveis ou credenciais no SQLite, logs, URL ou argumentos de CLI. `.env.example` não contém valores reais. Eventos de aprovação não carregam segredos ou payloads privados completos.

## Dashboard local

Bind em loopback `127.0.0.1`; autenticação local com segredo aleatório; validação de Host/Origin, CSRF e CORS restrito; cookies seguros. Porta 3199 preferida, fallback sem encerrar processo externo. Não expor API remota por padrão. Service worker não armazena dados sensíveis.

## Dados, backup e Cloud

SQLite e vault locais por padrão. Sync futuro é opt-in, apenas para dados selecionados, com E2EE e chaves sob controle do usuário. Segredos de provedores não sincronizam por padrão. Revogação não apaga cópias offline e perda das chaves sem recuperação escolhida torna dados cifrados irrecuperáveis. Consultar [ADR-0020](../adr/0020-cloud-identity-e2ee-and-pro-activation.md) e [documento Cloud](../08-cloud-sync-e-monetizacao.md). Terceiros (Claude/Codex) podem processar dados enviados a eles conforme seus próprios termos.

## Plugins

Exibir origem, versão fixada e permissões; verificar manifesto/assinatura quando suportado; exigir aprovação antes de instalar; isolar e permitir revogação. Conteúdo de skill não altera política de segurança.

## Validação pendente

Testes adversariais de prompt injection, CSRF, traversal de caminho, symlinks, escalada de permissões, cancelamento e vazamento de segredo. Cloud requer validação independente de E2EE, concorrência, revogação e recuperação. Este documento registra desenho, não implementação ou certificação.
