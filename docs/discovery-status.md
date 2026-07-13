# Discovery Status

**Observed at:** `2026-07-13T00:25:57-03:00`  
**Gate:** implementation and publication approved; execution now proceeds through implementation safety gates G1–G5

## Completed artifacts

- Product Contract: `docs/product-contract.md`
- Approved product decisions: `docs/product-decisions-v0.md`
- Command preview: `docs/capture-command-preview.md`
- Deterministic target: `fixture_target/app.py`
- Target tests: `tests/test_fixture_target.py`
- Wordlist: `fixtures/shared/arjun-minimal.txt`
- Manifest templates: `fixtures/templates/`
- URL/identity policy: `docs/url-canonicalization-v0.md`
- Machine-readable canonicalization vectors: `fixtures/canonicalization/v0/vectors.json`
- JSON Schema Draft 2020-12: `schemas/v0/`
- Schema semantics: `docs/schema-v0.md`
- Executable references: `reference/canonicalization_v0.py`, `reference/build_example_v0.py`
- Fixture-derived handoff: `examples/handoff-web-blackbox-v0/`
- Pipeline/approval contract: `docs/pipeline-v0.md`
- Threat model: `docs/threat-model.md`
- Go/Python spike ADR: `docs/adr/0001-language-and-distribution.md`
- Agent benchmark and metrics: `benchmarks/agent-handoff-v0.{md,json}`
- Competitive baseline and score: `docs/competitive-baseline-v0.md`, `benchmarks/competitive-baseline-v0.json`
- Operator failure-path preview/harness: `docs/failure-path-capture-preview-v0.md`, `scripts/capture-failure-path.sh`
- Compact agent front door: generated `CONTEXT.md` plus `normalized/agent-view.jsonl`
- Benchmark v1 protocol: `benchmarks/agent-handoff-v1-protocol.md`
- Public repository baseline: `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`, `.gitignore`
- Public fixture policy and compatibility matrix: `docs/fixture-policy.md`, `docs/compatibility-matrix-v0.md`

## Verified target evidence

Test command:

```bash
python3 -m unittest tests.test_fixture_target -v
```

Observed result:

```text
Ran 15 tests in 0.581s
OK
```

Real process verification:

- listener: `127.0.0.1:18080`;
- process: Python fixture target;
- `/healthz`: HTTP 200 with deterministic JSON;
- GET parameter classification: verified;
- POST form classification: verified;
- POST JSON classification: verified;
- redirect: HTTP 302 to internal URL;
- rate limiting: `200, 200, 429`;
- shutdown: port 18080 confirmed closed.

Safety behavior:

- non-loopback bind raises `ValueError` before socket creation;
- target has no external callbacks;
- external URLs embedded in content use `.invalid`;
- no active scanner was executed against an external site; only operator-controlled passive GAU archive queries were made for an owned domain.

## Verified tools

| Tool | Version | Path/wrapper | State |
|---|---:|---|---|
| Katana | v1.6.1 | `tools/bin/katana` | scoped loopback JSONL fixture validated |
| GAU | 2.2.4 | `tools/bin/gau` | passive fixtures validated: canonical text + JSON regression |
| Arjun | 2.2.7 | `tools/bin/arjun` | GET/POST/JSON/zero loopback fixtures validated |

Katana v1.6.1 required Go >=1.25.7; Go's toolchain switching installed/used Go 1.25.12 for the build while the host command initially reported Go 1.24.4.

## Arjun isolation finding

Hermes exports:

```text
PYTHONPATH=[LOCAL_HERMES_SOURCE]
```

That path made pip report unrelated Hermes dependency warnings from inside the new venv. The venv itself had `include-system-site-packages=false`.

Validation with `PYTHONPATH` removed returned:

```text
No broken requirements found.
hermes-agent: package not found
arjun=2.2.7
python=3.13.5
```

`tools/bin/arjun` therefore clears `PYTHONPATH` and `PYTHONHOME` before executing the isolated CLI.

## GAU fixture result

The operator executed passive GAU captures against an owned domain. No active scanner was run against the public site.

Completed cases:

- `GAU-APEX-SUBS-TEXT`: three sanitized records, two unique URLs, one duplicate;
- `GAU-JSON-EXTENSIONLESS-DROP`: release regression with zero-byte JSON output;
- private raw preserved with manifests and verified checksums.

Confirmed GAU 2.2.4 pitfalls:

- JSON mode drops extensionless URLs;
- output paths append across executions;
- provider errors can still result in exit code zero;
- provider identity is not retained per native URL line.

Contract: `docs/tool-contract-matrix-v0.md`.

## Local Katana and Arjun fixture result

The operator explicitly authorized Katana and Arjun only against `127.0.0.1:18080`. The target was verified as loopback-only before execution and the port was confirmed closed afterward.

Completed cases:

- `KAT-NORMAL-MINIMAL`: six valid JSONL records, six unique endpoints, all GET/200, external `.invalid` link excluded by scope;
- `ARJUN-GET-FOUND`: detected `q`; ground-truth `debug` was a false negative;
- `ARJUN-POST-FORM-FOUND`: detected complete ground truth `id`, `name`;
- `ARJUN-JSON-FOUND`: detected `filter`, `id`; ground-truth `debug` was a false negative;
- `ARJUN-ZERO`: exit zero, no native `-oJ` file, explicit zero-result message in stdout.

All private and public fixture sets have verified SHA-256 manifests. Public fixtures passed sensitive-string scanning and structural invariants. The target test suite remains 15/15 green.

Contract: `docs/tool-contract-matrix-v0.md`.

## Canonicalization, schema and handoff result

Canonicalization v0 now preserves raw URLs while separating observation URL identity from endpoint route identity. Query ordering/repetitions remain observable, fragments are excluded, methods are identity-bearing, GAU method stays unknown, and Arjun `JSON` maps to POST/JSON without losing the source label.

Schema v0 now provides strict Draft 2020-12 contracts for Run, ToolExecution, Asset, Endpoint, Parameter, Observation, Evidence, Relationship and handoff manifest. `auth_context_id` is required but nullable and accepts only opaque `authctx_...` identifiers; raw credentials are rejected.

The deterministic handoff example contains:

- 84 normalized records;
- 6 tool executions;
- 3 assets;
- 11 endpoints;
- 7 parameters;
- 17 observations;
- 15 evidence records;
- 24 relationships;
- 16 selected raw artifacts.

Consolidated offline validation:

```text
Ran 37 tests in 0.808s
OK
MANIFEST_FILES_OK=27
NORMALIZED_RECORDS=84
LOOPBACK_TARGET_PORT_18080=CLOSED
```

Every entry in `examples/handoff-web-blackbox-v0/checksums.sha256` also verified successfully.

## Compact handoff v1 result

The deterministic front door now contains the common factual answers, explicit gaps and all 15 resolvable Evidence IDs without raw drilldown. A derived, non-authoritative endpoint projection is included at `normalized/agent-view.jsonl`.

```text
Ran 39 tests in 0.890s
OK
CONTEXT_BYTES=7064
RAW_BYTES=9332
CONTEXT_REDUCTION_PERCENT=24.3
AGENT_VIEW_ROWS=11
MANIFEST_FILES_OK=28
DETERMINISTIC_REBUILD=YES
PORT_18080=CLOSED
FAKE_PROCESS_LEAKS=0
```

Sensitive-string scanning of the public handoff returned zero matches for credentials, authorization/cookie values, private keys and local home paths. The isolated three-condition benchmark v1 passed all 7 rules: 15/15 COMPACT Evidence IDs valid, 24.3% fewer unique input bytes and 75.0% fewer API calls than RAW.

## Autonomous gate results

- Pipeline DAG/two approval digests: complete — `docs/pipeline-v0.md`.
- Threat model: complete with 15 threat classes — `docs/threat-model.md`.
- Stack ADR: Go accepted; both equivalent spikes passed; no material blocker — `docs/adr/0001-language-and-distribution.md`.
- Competitive position: no pivot; BBOT 46.4%, reconFTW 28.6%, both below the 80% threshold — `docs/competitive-baseline-v0.md`.
- Agent benchmark v1: `PASS` 7/7 — COMPACT answered all ten questions from 7,064 bytes, cited 15/15 valid Evidence IDs, used 24.3% fewer bytes and 75.0% fewer calls than RAW — `benchmarks/agent-handoff-v1.md`.
- Failure-path active captures: FP-013/FP-014/FP-015 operator-executed on bounded loopback, checksum-verified, reviewed, sanitized and covered by regression tests. Katana interruption is partial; Arjun interruption is unknown/partial; Arjun request-timeout is tool-error/failed — `docs/failure-path-capture-preview-v0.md`.
- Final implementation plan with G0–G5 stop gates: `.hermes/plans/2026-07-13_002250-reconctx-mvp-implementation.md`.

Hermes did not execute a scanner or competitive framework. The operator executed the three bounded loopback failure-path cases. Current safety state is `PORT_18080=CLOSED` and no harness/fixture process remains.

## Current blockers

Operator approval was recorded at `2026-07-13T12:50:05-03:00` for implementation and publication. GitHub push/publication still waits for the plan's code, security, fixture and release verification gates.
