# Recon Context Compiler schemas v0

JSON Schema Draft 2020-12 contracts for `schema_version: reconctx/v0`.

## Entrypoints

- `record.schema.json` — validates any normalized JSONL record.
- `handoff-manifest.schema.json` — validates `manifest.json` for a packaged handoff.
- `arjun-candidate.schema.json` — validates each redacted `arjun-candidates.jsonl` decision.
- `common.schema.json` — shared IDs, diagnostics, scope, artifacts and locators.

## Record schemas

- `run.schema.json`
- `tool-execution.schema.json`
- `asset.schema.json`
- `endpoint.schema.json`
- `parameter.schema.json`
- `observation.schema.json`
- `evidence.schema.json`
- `relationship.schema.json`

Normative semantics are documented in:

- `docs/schema-v0.md`
- `docs/url-canonicalization-v0.md`
- `docs/tool-contract-matrix-v0.md`

The production compiler embeds these files and resolves their `$ref` values from an offline allowlisted loader. `tests/test_schema_v0.py` provides an independent offline-registry validation; neither path fetches remote schemas.
