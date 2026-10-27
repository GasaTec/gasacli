# 08 — Cloud, sincronização e monetização

## Modelo
Mesmo monorepo MIT para Community e Cloud. Community: orquestração completa local e gratuita. Cloud: serviço pago opcional por hospedagem de sync, backup criptografado, gerenciamento de dispositivos e painel web. Código MIT pode ser auto-hospedado; cobrança não concede exclusividade sobre código.

## Arquitetura planejada
CLI → sync-protocol → cloud-api → armazenamento de payload criptografado.
cloud-web administra conta, plano e dispositivos. Dados de cobrança separados de conteúdo sincronizado. O core não depende do serviço.

## O que sincronizar
Configurações não secretas, prompts personalizados, metadados de sessões e tarefas, e conteúdo do segundo cérebro somente se habilitado. Código-fonte não deve ser enviado automaticamente. Tokens dos provedores nunca sincronizados por padrão.

## Criptografia
Projetar E2EE com chaves sob controle do usuário, autenticação de dispositivos, rotação, revogação e recuperação documentada. HTTPS e criptografia em repouso não substituem E2EE. Definir estratégia de conflitos, versionamento, quotas e retenção antes de implementar.

## Planos
Community gratuito; Pro por sync gerenciado; Teams futuramente com políticas e administração. Preço é hipótese comercial e precisa validar custos, suporte, tributos, storage e conversão.

## Billing
Backend mantém somente dados necessários à conta, assinatura, status e direitos. Pagamento via provedor apropriado; não armazenar dados completos de cartão. Webhooks idempotentes, auditoria e cancelamento. Política de privacidade e LGPD antes de produção.

## Entregas
Fase 1 core local; fase 2 protocolo de sync self-hostable; fase 3 serviço gerenciado e billing; fase 4 equipes.
