# Recon Context Compiler schemas v0

JSON Schema Draft 2020-12 contracts for `schema_version: reconctx/v0`.

## Entrypoints

- `record.schema.json` — validates any normalized JSONL record.
- `handoff-manifest.schema.json` — validates `manifest.json` for a packaged handoff.
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

Validation uses an offline registry in `tests/test_schema_v0.py`; no remote `$ref` fetch is required.
