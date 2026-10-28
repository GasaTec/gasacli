# 08 — Cloud, sincronização e monetização

> **Estado:** desenho de produto e protocolo; Cloud, sync e ativação Pro não estão implementados. Decisões e limites deste documento não descrevem um serviço disponível. Veja também [ADR-0020](adr/0020-cloud-identity-e2ee-and-pro-activation.md).

## Separação Community, Sync e Pro

- **Community:** execução e dados locais continuam disponíveis sem conta, conexão ou serviço Gasatec.
- **Sync:** capacidade opcional para replicar dados selecionados e cifrados entre dispositivos. A sincronização gerenciada pode ter cobrança por hospedagem e operação.
- **Pro:** direito de uso de serviços gerenciados cobertos pelo plano vigente. Não desbloqueia orquestração local, histórico local, backup local nem execução de agentes.
- **Billing:** processa assinatura e entitlement. Metadados de conta, pagamento e direito de uso ficam separados dos objetos de sync cifrados.

O Cloud não inclui assinaturas de provedores de IA nem sincroniza credenciais desses provedores por padrão. O cliente permanece utilizável offline; indisponibilidade do serviço pode interromper sync/gestão remota, não trabalho local.

## Identidade e dispositivos

Autenticação de conta é distinta da chave E2EE. Cada instalação cria uma identidade de dispositivo e chaves localmente; proteger segredos no cofre do SO quando disponível. Não usar fingerprint de hardware como única identidade. Cadastro de dispositivo requer sessão autenticada e confirmação explícita de dispositivo já autorizado ou segredo de recuperação. Mostrar nome, data de cadastro e último contato conhecido, além de oferecer revogação.

Revogar bloqueia novos tokens e operações de sync daquele dispositivo após propagação do serviço. Não apaga cópias locais, conteúdo já decifrado ou ciphertext previamente baixado. A tela deve explicar esse limite antes da confirmação. A propagação imediata e o prazo máximo de invalidação são **NÃO VERIFICADOS** até serem definidos e testados no serviço.

## Protocolo E2EE proposto

### Escopo e transporte

Sincronizar somente coleções selecionadas pelo usuário: configurações não secretas, prompts, metadados/histórico selecionado e segundo cérebro apenas quando ativado. Código-fonte, tokens, cookies, variáveis de ambiente, entradas do cofre do SO e segredos de projeto ficam excluídos por padrão. HTTPS protege o transporte; não substitui a cifragem no cliente antes do upload.

Cada objeto deve ter identificador opaco, versão de formato, versão/chave de cifragem, revisão causal, tamanho cifrado e ciphertext autenticado. Evitar nomes de arquivo ou metadados que revelem conteúdo. O servidor roteia, limita e retém blobs cifrados sem receber chaves de conteúdo. Metadados mínimos inevitáveis (conta, dispositivo, volume/horário de atividade e tamanho aproximado) devem ser documentados no aviso de privacidade; o grau de vazamento atual é **NÃO VERIFICADO** até existir protótipo de protocolo.

### Ciclo de chaves e recuperação

1. Criar material de cifragem no dispositivo e mantê-lo protegido localmente; derivar chaves com KDF e primitivas aprovadas após revisão criptográfica. Algoritmos, parâmetros e bibliotecas ainda precisam ser selecionados e auditados.
2. Vincular novos dispositivos por troca autenticada entre dispositivo autorizado e novo dispositivo, ou por uma chave de recuperação que o usuário exportou e guardou separadamente. A conta do serviço não decifra dados.
3. Permitir rotação com recriptografia gradual/versões, identificando dispositivos pendentes. Definir comportamento de exclusão de chave antiga e objetos que permanecem em backups antes da implementação.
4. Se perder todas as chaves/dispositivos e não tiver material de recuperação, dados E2EE ficam irrecuperáveis. Suporte, troca de senha da conta e redefinição de billing não recuperam conteúdo.
5. Qualquer escrow que permita recuperação pelo operador altera o modelo de confiança e não está aprovado. Exigiria consentimento separado, revisão de segurança e explicação explícita de quem pode decifrar.

### Revisões e conflitos

Uploads usam revisão monotônica/idempotency key e identificam os pais/revisões observadas. Um cursor por dispositivo permite retomar transferência interrompida sem duplicar eventos. Alterações concorrentes em texto ou config são preservadas como versões conflitantes; apresentar comparação e deixar a resolução ao usuário. Não resolver silenciosamente por “última gravação vence”. Eventos de execução/histórico podem ser mesclados como itens imutáveis se o formato confirmar identidade e duplicação; isso requer especificação e teste antes de implementação.

Uma revogação impede operações futuras após propagação, mas não invalida cópias offline. Apagar conta deve cancelar entitlement e agendar exclusão dos blobs e metadados segundo retenção publicada. Backups e restauração precisam declarar janela de expurgo; prazo ainda não definido.

## Quotas, retenção e custos

Não há quotas ou preço aprovados. Antes de vender sync, publicar limites mensuráveis por plano para bytes cifrados, número de dispositivos, tamanho de objeto, frequência/volume de operações e retenção de versões. O cliente deve mostrar consumo e bloquear/pausar novas gravações de forma previsível perto do limite, sem apagar dados locais. A política deve explicar excedentes, exportação e remoção. Valores só podem ser definidos após estimativa de storage, tráfego, backups, suporte, tributos e margem; não inventar valores neste desenho.

## Self-hosting

O protocolo e cliente devem permitir apontar para endpoint compatível operado pelo usuário, sem dependência obrigatória de identidade, billing, DNS ou API da Gasatec. Publicar especificação versionada, formatos, migrações, configuração, backup/restore, requisitos operacionais e limites de compatibilidade. O servidor próprio pode escolher quotas, retenção e identidade; clientes devem indicar claramente o operador e suas políticas. O código Cloud futuro no monorepo segue MIT conforme [ADR-0005](adr/0005-open-source-and-cloud.md); serviço gerenciado cobra hospedagem e operação, não licença exclusiva.

## Billing e ativação Pro

Compra e gestão de assinatura ficam em serviço separado do protocolo de conteúdo. O serviço verifica confirmação por mecanismo autenticado servidor-a-servidor, processa eventos com idempotência, registra auditoria e só então emite entitlement. Redirect do navegador sozinho não comprova pagamento. Não armazenar dados completos de cartão.

Máquina de estado proposta para código de ativação:

```text
issued -> redeemed -> entitlement_active -> grace -> expired
   |          |                 |             |
   +-> expired+-> revoked       +-> revoked   +-> revoked
```

- `issued`: código aleatório, expiração curta, armazenado com hash; não é chave de API.
- `redeemed`: transição atômica e única associada à conta autenticada; repetição falha sem revelar a quem o código pertence.
- `entitlement_active`: direito associado à conta e dispositivos permitidos no plano; cliente recebe credencial renovável guardada no cofre do SO.
- `grace`: falha de rede temporária permite validar o último entitlement por janela limitada e configurável, ainda a definir. Não estender grace por reinicialização ou alteração do relógio local sem política anti-replay.
- `expired` ou `revoked`: serviço gerenciado deixa de ser autorizado; remover credenciais revogadas quando online e voltar a Community sem remover dados locais.

Cancelamento, estorno e fraude precisam de regra explícita de entitlement; não inferir comportamento comercial antes de validar fluxo com provedor. Códigos não devem aparecer em argumentos, URL ou logs.

## Provedor de pagamento: alegações não verificadas

**UNVERIFIED:** InfinitPay foi mencionado como candidato, mas suporte atual a recorrência, webhooks autenticados, idempotência, cancelamento/estorno, API, países/moedas e testes sandbox não foi confirmado. Antes de escolher, verificar documentação primária vigente e produto/região exatos, testar webhook assinado e reentrega em sandbox, reconciliar eventos e registrar data/evidência. Não anunciar integração ou capacidade como existente sem essa verificação. A mesma checagem é necessária para qualquer outro provedor.

## Privacidade e LGPD

Antes de produção: mapear finalidade e base legal por dado; minimizar coleta; publicar controlador/operadores, retenção, subprocessadores, transferências internacionais, canal do titular e processo de incidente; definir acesso, correção, exportação e exclusão da conta e metadados. Explicar que E2EE limita recuperação e que metadados operacionais podem continuar visíveis. Fazer revisão jurídica e de segurança sobre LGPD e contratos; este documento não é parecer jurídico nem afirma conformidade. A lista de subprocessadores e localidades de hospedagem é **NÃO VERIFICADA** até seleção e contrato do provedor.

## Verificações antes de implementação/lançamento

1. Revisão independente de protocolo, modelo de ameaça, KDF, primitivas, bibliotecas e vetores de teste.
2. Testes com perda/rotação de chave, dispositivo offline/revogado, reenvio, concorrência, conflito, recuperação e restauração.
3. Definir e testar limites de propagação de revogação, retenção, expurgo de backups, quotas e política de grace.
4. Verificar fornecedor de pagamento em documentação primária e sandbox; manter evidências versionadas e datadas.
5. Concluir revisão de privacidade/LGPD, contratos e subprocessadores antes de receber dados reais.
6. Não declarar Cloud, E2EE, recuperação ou billing prontos por esta especificação: são critérios de implementação e validação futuros.
