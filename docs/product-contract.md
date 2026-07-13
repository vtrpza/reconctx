# Product Contract v0.1

**Status:** Approved for discovery  
**Approved product decisions:** `docs/product-decisions-v0.md`  
**Product:** Operator-Run Recon Context Compiler  
**Scope:** primeiro vertical slice `web-blackbox`

## Product promise

O pentester executa uma única CLI local que coordena ferramentas de recon, preserva evidências e produz um handoff estruturado para um agente analisar posteriormente.

```text
operator → plan → approve → execute tools → normalize → compile → handoff → agent
```

## Control boundary

- O pentester inicia toda execução.
- A CLI mostra o plano e os comandos efetivos antes de atividade.
- Uma aprovação autoriza GAU e Katana.
- Uma segunda aprovação autoriza Arjun e sua lista de targets.
- O agente não recebe mecanismo de execução.
- O produto não cria MCP, daemon, API, callback ou tool agent-callable.
- Quando o agente precisar de mais dados, ele sugere uma próxima execução ao operador.

## MVP tools

| Tool | Papel | Classe de atividade |
|---|---|---|
| GAU | URLs históricas de providers externos | passiva |
| Katana | crawl de endpoints atualmente observáveis | ativa, bounded |
| Arjun | descoberta de parâmetros em endpoints aprovados | ativa, segunda aprovação |

## Canonical workflow

1. Receber target, seed URLs, scope e profile.
2. Verificar tools, paths, versões e workspace.
3. Renderizar plano completo sem executar atividade.
4. Receber aprovação do operador.
5. Executar GAU e Katana, potencialmente em paralelo.
6. Preservar stdout, stderr, exit code, output nativo, versão e comando.
7. Normalizar URLs sem destruir representações raw.
8. Correlacionar observações históricas e atuais sem fundi-las semanticamente.
9. Aplicar scope e candidate policy.
10. Mostrar a fila do Arjun e os comandos efetivos.
11. Receber segunda aprovação.
12. Executar somente os candidatos aprovados.
13. Compilar handoff Markdown + JSONL.
14. Encerrar; nenhuma conexão runtime com o agente permanece.

## Inputs

- domínio raiz;
- seed URLs;
- `scope.yaml` versionado;
- profile;
- flags adicionais explicitamente permitidas;
- limites de rate, concurrency, timeout e targets;
- diretório de workspace.

## Outputs

```text
handoff/<run-id>/
├── README.md
├── CONTEXT.md
├── manifest.json
├── checksums.sha256
├── normalized/
│   ├── records.jsonl
│   ├── runs.jsonl
│   ├── tool-executions.jsonl
│   ├── assets.jsonl
│   ├── endpoints.jsonl
│   ├── parameters.jsonl
│   ├── observations.jsonl
│   ├── relationships.jsonl
│   ├── evidence-index.jsonl
│   └── arjun-candidates.jsonl
└── raw/ or relative raw references
```

O handoff precisa ser legível sem a CLI instalada.

## Evidence contract

Todo fato normalizado relevante deve apontar para:

- tool;
- tool version;
- adapter/schema version;
- run ID;
- comando redigido;
- timestamp;
- raw artifact;
- record/line ou boundary equivalente;
- scope decision;
- semantic state;
- `auth_context_id` opaco/nullable; nunca token, cookie ou header bruto.

Estados semânticos mínimos:

- `observed`;
- `historical`;
- `inferred`;
- `bruteforced`;
- `user_supplied`.

## Failure contract

- Falha parcial não elimina resultados válidos.
- Tool ausente falha no preflight da fase afetada.
- Formato desconhecido preserva raw e gera `unsupported_format`.
- Zero resultados é sucesso com contagem zero quando a tool conclui normalmente.
- Ctrl-C encerra filhos, preserva artifacts parciais e marca `interrupted`.
- Out-of-scope é registrável, mas nunca agendado para atividade.
- Handoff parcial declara lacunas explicitamente.

## Safety contract

- Subprocessos sem shell interpolation por padrão.
- Tool path resolvido e registrado.
- Scope aplicado antes de agendar atividade.
- Rate/concurrency bounded.
- Segredos e PII fora do handoff por padrão.
- Conteúdo do target rotulado como não confiável.
- Escrita limitada ao workspace e sem seguir symlinks inseguros.
- Nenhum scan externo durante desenvolvimento de fixtures; usar target local/controlado.

## Discovery defaults

Defaults provisórios, revisáveis após fixtures:

```yaml
katana:
  depth: 2
  rate_limit_per_second: 2
  concurrency: 1
  parallelism: 1

arjun:
  approval: required
  max_targets: 25
  rate_limit_per_second: 1
  threads: 1

handoff:
  mode: compact
  token_budget: 12000
  raw_policy: referenced
```

## Candidate policy v0 for Arjun

Excluir:

- out-of-scope/unknown;
- extensões estáticas;
- duplicates canônicos;
- paths excluídos;
- histórico não observado atualmente, salvo opt-in;
- itens além de `max_targets`.

Priorizar:

1. observado pelo Katana;
2. query parameters existentes;
3. APIs;
4. multi-source;
5. sem extensão estática;
6. método/location suportado.

A fila precisa explicar inclusão, exclusão e ranking.

## Non-goals do MVP

- exploração;
- findings automáticos;
- severity automática;
- autenticação/HAR/Burp;
- headless obrigatório;
- dashboard;
- execução distribuída;
- plugin ecosystem;
- embeddings/vector DB;
- scanner autônomo;
- integração runtime com agente;
- suporte a dezenas de ferramentas.

## Success definition

O MVP é útil quando um agente, recebendo somente o handoff:

- distingue histórico de atual;
- mapeia assets, endpoints e parâmetros;
- cita evidence IDs válidos;
- declara falhas e lacunas;
- não inventa findings;
- usa menos contexto que o raw sem perder factualidade relevante.

## Change rule

Qualquer fixture real que contradiga este contrato abre uma decisão explícita. O contrato não deve ser contornado por adapters ad hoc.
