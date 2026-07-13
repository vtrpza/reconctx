# Normalized Record Schema v0

**Schema ID:** `reconctx/v0`  
**Status:** approved discovery contract  
**Canonicalization:** `url-canonicalization/v0`  
**JSON Schema dialect:** Draft 2020-12

## Purpose

Schema v0 represents evidence-backed reconnaissance without collapsing raw occurrences, tool semantics, or confidence boundaries. It was derived from the sanitized GAU 2.2.4, Katana v1.6.1, and Arjun 2.2.7 fixtures.

The executable schemas are under `schemas/v0/`. `record.schema.json` is the entrypoint for normalized JSONL records; `handoff-manifest.schema.json` validates a packaged handoff manifest.

## Record model

| Record | Identity | Responsibility |
|---|---|---|
| `Run` | occurrence ID | approved scope, policy, overall status, tool executions, warnings and gaps |
| `ToolExecution` | occurrence ID | tool/version, redacted argv, timing, exit code, semantic status, coverage and artifacts |
| `Asset` | deterministic SHA-256 | canonical origin/host/IP/URL plus scope decision |
| `Endpoint` | deterministic SHA-256 | method or unknown method plus canonical route URL |
| `Parameter` | deterministic SHA-256 | endpoint, case-sensitive name and location |
| `Observation` | evidence-derived SHA-256 | historical/current/bruteforced fact emitted by one tool execution |
| `Evidence` | artifact/locator SHA-256 | immutable artifact digest and line, JSON pointer, byte range or whole-file locator |
| `Relationship` | edge SHA-256 | explicit, evidence-backed connection between records |

Every normalized record carries `schema_version: reconctx/v0`. Entity records do not replace observations, and observations do not replace raw evidence.

## Strictness

Entity schemas use `additionalProperties: false`. New top-level fields require a schema revision. Controlled extension is limited to `Relationship.attributes`, whose values are JSON scalars or arrays of scalars.

## Authentication context

`ToolExecution.auth_context_id` and `Observation.auth_context_id` are required but nullable. Black-box fixtures use `null`; authenticated collection uses an opaque identifier matching `authctx_<id>`. Tokens, cookies and authorization headers are never valid values. The identifier correlates observations made under the same external secret reference without embedding that secret.

JSON Schema validates structure and local field constraints. The following global invariants require an additional integrity pass and are covered by `tests/test_example_v0.py`:

- IDs are globally unique;
- all `run_id`, entity and evidence references resolve;
- relationship endpoints exist;
- aggregate `observation_ids` and `evidence_ids` exist;
- manifest file hashes match packaged files.

## URL and identity boundary

The normative URL rules are in `docs/url-canonicalization-v0.md`.

Key consequences:

- query and fragment do not enter Endpoint identity;
- method is part of Endpoint identity;
- GAU method is `null` with `method_known: false`, not guessed as GET;
- Arjun `JSON` maps to HTTP POST with parameter location `json`;
- path case, repeated slashes and trailing slash remain significant;
- duplicate raw occurrences remain separate Evidence/Observation records.

## Tool execution status

Process completion and semantic coverage are separate:

| Field | Values |
|---|---|
| `status` | `success`, `success_zero`, `partial`, `failed`, `interrupted`, `timed_out`, `skipped`, `unsupported_format` |
| `coverage` | `complete`, `partial`, `zero`, `unknown` |

Fixture-derived rule: Arjun can exit 0, report no parameters, and omit its native `-oJ` file. This is represented as `status: success_zero`, `coverage: zero`, an absent native artifact summary, and stdout Evidence. It is not a claim that no parameter can exist.

GAU provider failures can also be masked by exit code 0. Adapters therefore derive semantic status from process state, native output, stderr, and provider diagnostics rather than exit code alone.

## Observation types

| Type | Meaning |
|---|---|
| `historical_url` | URL returned by an archive/provider query; current reachability remains unknown |
| `http_response` | request/response observed during the recorded execution |
| `parameter_discovery` | parameter candidate from URL evidence, brute force, inference or user input |
| `zero_result` | a normally completed tool phase emitted no result |
| `tool_warning` | structured adapter/tool diagnostic |

Semantic states are `observed`, `historical`, `inferred`, `bruteforced`, and `user_supplied`.

## Evidence locators

Evidence contains an artifact with relative path, media type, size, SHA-256 and sanitization flag. A locator is exactly one of:

- `whole_artifact`;
- `line_range`;
- `json_pointer`;
- `byte_range`.

Raw/target content is untrusted data. A locator is a citation boundary, not an instruction boundary.

## Handoff layout v0

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
│   ├── evidence-index.jsonl
│   └── relationships.jsonl
└── raw/
```

`normalized/records.jsonl` is the complete stream. Split JSONL files are deterministic projections for human/tool convenience.

## Fixture-derived example

`examples/handoff-web-blackbox-v0/` contains a deterministic, sanitized composite generated by `reference/build_example_v0.py`.

It currently contains:

- 1 Run;
- 6 ToolExecutions;
- 3 Assets;
- 11 Endpoints;
- 7 Parameters;
- 17 Observations;
- 15 Evidence records;
- 24 Relationships.

The example demonstrates Katana+Arjun correlation at `GET /api/search`, GAU duplicate preservation, unknown GAU methods, and Arjun `success_zero`.

## Validation

```bash
.tools/schema-venv/bin/python -m unittest discover -s tests -v
cd examples/handoff-web-blackbox-v0
sha256sum -c checksums.sha256
```

Compatibility requires passing both JSON Schema validation and the machine-readable URL vectors. Changing canonical URL or deterministic ID output requires a new canonicalization policy version.
