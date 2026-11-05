# ADR-0021 — Escritório 3D no localhost e backend Orca preferencial

Data: 2026-11-04.

Status: direção de produto solicitada pelo usuário; desenho técnico proposto, implementação ainda não realizada.

## Contexto

O GasaCLI já oferece CLI/TUI, servidor Go local, sessões Claude/Codex e motor nativo. A experiência desejada passa a ser um escritório 3D no navegador, com comunicação visível e chat para time, coordenador ou qualquer integrante. O usuário rejeitou manter a aparência de Orca/IDE no localhost e forneceu 12 imagens de referência.

O Orca dispõe de CLI para workspaces, agentes, tarefas, workers supervisionados, mensagens e gates; também tem plugins. O Openbot oferece referência de organização com integrantes persistentes, canais e filas. Recriar ambos os motores aumentaria trabalho e risco de divergência.

## Decisão proposta

1. Tornar o escritório 3D a interface principal do localhost, com identidade visual própria e controles integrados à cena.
2. Manter Go, armazenamento local, políticas, distribuição e dados existentes do GasaCLI.
3. Usar Orca como backend preferencial dos novos runs do escritório, via adaptador versionado da CLI pública.
4. Fixar um backend por run; manter o motor nativo para compatibilidade e modo independente explícito.
5. Exigir registro e decisão de coordenação antes de iniciar trabalho pedido diretamente a um agente.
6. Separar decisão do coordenador, aprovação humana, aceitação do host e conclusão verificada.
7. Reutilizar plugins no host conforme capacidades suportadas; não criar loader/marketplace paralelo nem presumir paridade entre plugin API e CLI.
8. Usar Openbot como referência de produto, implementando contratos próprios sem importar seu código/asset sob licença incompatível com a distribuição pretendida.
9. Tratar 3D como projeção de eventos observados: mesa, monitor, telefone, reunião e café têm significado, sem conceder autoridade ao frontend.
10. Não executar GasaCLI, iniciar localhost ou testar o produto nesta etapa; aguardar solicitação do usuário.

## Relação com decisões existentes

- ADR-0001 e ADR-0007: motor nativo continua válido para modo nativo; a preferência Orca é nova para os runs do escritório.
- ADR-0002: integração inbound, GasaCLI como agente hospedado, continua separada e não é requisito da primeira ponte outbound. Campos reservados de plugins não comprovam registro de harness.
- ADR-0006: preserva backend Go; módulo 3D web não muda a linguagem do motor.
- ADR-0009 e ADR-0018: preserva localhost/distribuição/segurança; substitui a composição visual principal de dashboard por escritório.
- ADR-0015: para runs nativos SQLite mantém autoridade existente; para runs Orca, estado de worker/dispatch pertence ao Orca e SQLite GasaCLI guarda intenções, decisões, vínculos e projeções.
- ADR-0019: políticas continuam no backend; autonomia visual não amplia permissões do provider.
- `docs/design/DESIGN.md`: marca, legibilidade e acessibilidade permanecem; composição IDE/abas/sidebar dos prompts anteriores deixa de ser alvo para o escritório.

## Consequências

Menor duplicação de drivers e supervisão; maior dependência de contrato e disponibilidade do Orca. A ponte deve provar identidade do coordenador, recuperação de operações ambíguas e cobertura das permissões em cada provider.

Hooks de plugins possuem limites por entrada, provider e host. Não garantem controle de tudo que uma pessoa ou agente fizer fora do escritório. Compatibilidade é publicada por capacidade/versão e protegida por testes.

Voz, copiloto de tela e controle desktop são subentregas posteriores. Uso de assinaturas existentes é a direção do núcleo; nenhuma API cobrada por consumo será ativada para preencher lacunas sem configuração e autorização explícitas.

## Alternativas

Plugin exclusivo dentro do Orca: útil como complemento, insuficiente para garantir o localhost independente e toda a supervisão via API de plugins.

Fork do Openbot ou novo runtime: maior liberdade, mas duplica responsabilidades e exige avaliar a licença PolyForm Noncommercial do snapshot estudado. Não adotado como caminho principal.

## Especificação e aceite

Ver [índice do escritório](../escritorio-3d/README.md), [pesquisa](../escritorio-3d/01-pesquisa-e-evidencias.md), [referências](../escritorio-3d/07-requisitos-das-imagens.md) e [plano/matriz de validação](../escritorio-3d/06-entrega-e-validacao.md).
