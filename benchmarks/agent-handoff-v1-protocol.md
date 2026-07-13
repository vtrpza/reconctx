# Agent Handoff Benchmark v1 Protocol

**Purpose:** retest factuality and context efficiency after the compact front-door redesign.

## Conditions

Three isolated leaf agents receive the same ten questions and one source condition each:

1. `RAW`: only 16 selected raw fixture artifacts.
2. `NORMALIZED`: only the complete `normalized/records.jsonl` stream.
3. `COMPACT`: only the generated `CONTEXT.md`; no manifest, agent view, records or raw drilldown.

Inputs are copied to separate temporary directories before delegation. Source bytes are recorded before execution.

## Questions

1. Quantos assets/origins in-scope aparecem, e quais são atuais versus apenas históricos?
2. Quais endpoints foram observados pelo Katana nesta run?
3. Quais URLs vieram apenas de fontes históricas?
4. Quais entidades têm evidência em mais de uma source family?
5. Quais parâmetros foram descobertos pelo Arjun e em qual location/method?
6. Quais tools falharam, foram parciais ou tiveram coverage gaps?
7. Quais candidatos foram excluídos do Arjun e por quê?
8. Qual Evidence ID sustenta cada resposta? Se IDs não existirem na condição, declarar indisponível.
9. Quais lacunas devem orientar a próxima execução?
10. Há alguma vulnerabilidade confirmada?

## Required result

Each agent must return:

- answers to all ten questions;
- citations available in its source condition;
- exact files read;
- exact total bytes read or the strongest measurable proxy;
- unsupported-claim self-audit;
- explicit unknowns without inference from filenames when contents do not support them.

## Pass rules

- no historical/current confusion;
- no nonexistent Evidence ID;
- no finding claim;
- compact answers all common questions from `CONTEXT.md` alone;
- compact source bytes lower than RAW source bytes;
- compact does not make more tool/API calls than RAW;
- missing candidate-exclusion artifact is reported as a gap rather than invented.

## Grounding and review

Subagent self-audits are not accepted as proof. The parent validates cited Evidence IDs against canonical records and compares answers to fixture ground truth after completion.
