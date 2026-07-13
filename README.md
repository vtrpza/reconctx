# reconctx

> Evidence, not terminal noise.

`reconctx` is an operator-controlled reconnaissance evidence compiler. It preserves native tool evidence, normalizes observations without erasing provenance, and builds compact, portable handoffs for downstream analysis.

## Status

**Pre-implementation discovery.** The production runner does not exist yet and no production use is supported.

Validated so far:

- GAU 2.2.4, Katana v1.6.1 and Arjun 2.2.7 fixture contracts;
- `reconctx/v0` JSON Schemas and URL canonicalization;
- deterministic fixture-derived handoff and compact agent view;
- two digest-bound operator approval gates;
- process-control spikes in Go and Python;
- Go selected for the production CLI;
- independent compiler-first architecture; BBOT importer planned later.

See `docs/discovery-status.md` for current blockers.

## Thesis

Recon tools produce useful data, but their outputs commonly lose context when flattened into terminal logs or deduplicated lists. `reconctx` keeps three layers separate:

1. native artifacts and immutable execution evidence;
2. normalized entities, observations and relationships;
3. derived compact views for agents and humans.

Every material handoff claim should resolve to an Evidence ID and native locator. Historical URLs are not presented as currently observed. Parameter candidates are not findings. Missing coverage is explicit.

## Intended workflow

```text
plan → operator approval A → bounded GAU/Katana capture
     → offline normalize → deterministic Arjun candidate queue
     → operator approval B → bounded Arjun capture
     → offline compile → CONTEXT.md + normalized records + Evidence map
```

The operator executes all active tools. Compilation, import, validation and handoff generation remain offline.

## Non-goals for the MVP

- autonomous or agent-controlled scanning;
- exploitation or vulnerability validation;
- automatic findings, severity or confidence scoring;
- broad scanner orchestration comparable to reconFTW;
- dashboard/plugin ecosystem;
- distributed execution;
- mandatory SaaS services;
- Windows/macOS support;
- authenticated HAR/Burp workflows in the first slice.

## Discovery artifacts

- Product contract: `docs/product-contract.md`
- Approved decisions: `docs/product-decisions-v0.md`
- Pipeline and approvals: `docs/pipeline-v0.md`
- Threat model: `docs/threat-model.md`
- Tool contracts: `docs/tool-contract-matrix-v0.md`
- Canonicalization: `docs/url-canonicalization-v0.md`
- Schema: `docs/schema-v0.md`, `schemas/v0/`
- Stack ADR: `docs/adr/0001-language-and-distribution.md`
- Agent benchmarks: `benchmarks/agent-handoff-v0.md`, `benchmarks/agent-handoff-v1.md`
- Competitive baseline: `docs/competitive-baseline-v0.md`
- Fixture policy: `docs/fixture-policy.md`
- Compatibility matrix: `docs/compatibility-matrix-v0.md`
- Failure-path preview: `docs/failure-path-capture-preview-v0.md`
- Example handoff: `examples/handoff-web-blackbox-v0/`
- Final implementation plan: `.hermes/plans/2026-07-13_002250-reconctx-mvp-implementation.md`

`examples/handoff-web-blackbox-v0/` is the immutable input snapshot used by benchmark v1. Its recorded gaps describe that run at build time; current fixture coverage is tracked by `docs/discovery-status.md` and `docs/compatibility-matrix-v0.md`.

## Validation

```bash
.tools/schema-venv/bin/python -m unittest discover -s tests -v
cd examples/handoff-web-blackbox-v0
sha256sum -c checksums.sha256
```

Current materialized reference result: 39 tests, 28 manifest files, 11 compact agent-view rows and deterministic rebuild.

## Safety

- No external scan during automated discovery or CI.
- Active execution requires an exact plan, scope, limits and operator approval.
- Loopback captures are also operator-executed.
- Target/raw content is untrusted data, never instructions.
- Private raws are immutable; public fixtures are separately sanitized.
- Production implementation remains blocked until the acceptance exit gate and final operator approval.

See `SECURITY.md`, `CONTRIBUTING.md` and `docs/fixture-policy.md`.

## License

Apache License 2.0. See `LICENSE`.
