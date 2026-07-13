# ADR-0001: Go for the production CLI

- **Status:** Accepted; production implementation approved 2026-07-13
- **Date:** 2026-07-12
- **Decision owner:** operator, through `docs/product-decisions-v0.md`
- **Evidence:** `spikes/001a-python/`, `spikes/001b-go/`, `spikes/results/`

## Context

The product is a Linux-first, operator-run CLI that supervises heterogeneous subprocesses, streams and preserves raw output, normalizes evidence deterministically, and compiles a portable handoff. The approved policy selected Go unless an equivalent spike demonstrated a material blocker.

The stack decision is primarily about process correctness, distribution, environment isolation, adapter maintainability, and schema evolution. Raw speed is secondary for a workflow dominated by external tool and network latency.

## Feasibility questions

1. Can both stacks parse captured native JSONL and emit equivalent deterministic artifacts?
2. Can both expose a minimal local CLI without shell interpolation?
3. Can both preserve stdout/stderr while timing out a fake tool whose process tree ignores `SIGTERM`?
4. Does Go's distribution advantage introduce a material complexity blocker?

## Equivalent spike

Both implementations used only their standard libraries and the same inputs:

- Katana `v1.6.1` six-record fixture;
- `spikes/fake_tool.py`, which emits both streams, spawns a child, ignores `SIGTERM`, and hangs;
- the same assertions for records, URLs, context, timeout, streams, exit state, and duration;
- the same materialized output contract.

No scanner or network target was executed.

## Results

| Dimension | Python 3.13.5 | Go 1.24.4 |
|---|---:|---:|
| Behavioral tests | 3 passed | 3 passed |
| Test elapsed | 0.391 s | 0.354 s |
| Materialized records | 6 | 6 |
| Semantic artifact equivalence | yes | yes |
| `CONTEXT.md` byte equivalence | yes | yes |
| Timeout + TERM/KILL | passed | passed |
| Descendant leaks | 0 | 0 |
| Median compile microbenchmark | 110,686 ns/op | 76,128 ns/op |
| Production source in spike | 92 lines | 193 lines |
| External dependencies | 0 | 0 |
| Distribution artifact | Python runtime + source/bundle | 1.903 MiB stripped binary |
| SQLite availability | stdlib SQLite 3.46.1 | external driver required if adopted |

The Go microbenchmark was 1.454x faster (31.2% lower median latency), but the six-record file-heavy sample is too small to drive the decision. The relevant result is that both stacks satisfy the mechanics and Go has no feasibility blocker.

## Decision

Use **Go** for the production `reconctx` CLI.

Keep **Python** for:

- executable canonicalization/schema references;
- fixture builders and sanitizers;
- discovery and benchmark scripts;
- cross-implementation golden tests where useful.

The Python tooling is not a runtime dependency of the distributed CLI.

## Rationale

### Why Go

- one small self-contained binary aligns with local pentest distribution;
- avoids coupling the product runtime to Arjun's or the host's Python environment;
- explicit process groups, goroutines, contexts, and typed state machines fit supervised execution;
- typed schema/domain models reduce accidental semantic drift;
- standard-library streaming and JSONL paths were sufficient in the spike;
- performance/headroom is adequate without being the deciding factor.

### Why not Python for production

Python was fully viable and substantially more concise. It was not selected because distribution and environment isolation are recurring product concerns, not because of parsing performance. Shipping a venv/zipapp/frozen binary would add a packaging layer while still interacting with external Python tools.

### Why not rewrite the reference implementation now

The existing Python reference made the schema executable before the stack decision. Rewriting it before production approval would add no product evidence. Production Go must prove conformance against the same vectors and handoff fixtures.

## Consequences

### Positive

- Linux releases can be checksumable single binaries.
- Core runner and state model receive compile-time type checking.
- The CLI can remain dependency-light.
- External tools remain subprocesses and are never bundled.

### Costs

- Adapters and fixtures require more code than Python.
- Go needs explicit library decisions for UTS-46/IDNA, Draft 2020-12 validation, and optional SQLite.
- Cross-platform process semantics need later platform-specific implementations/tests.

### Risks to resolve in the implementation plan

1. Choose a UTS-46 implementation and run all canonicalization vectors against Go.
2. Decide whether production validates JSON Schema dynamically or validates typed records plus golden cross-checks.
3. Keep SQLite out of the canonical layer; if adopted, select a maintained rebuildable index driver after dependency review.
4. Implement Linux `openat2`/no-follow workspace safety or document the exact fallback.
5. Test process-group cleanup, signal races, output bounds, and resume with fake children before any real adapter.
6. Keep adapter environment policies explicit, especially for Python tools.

## Rejected alternatives

### Python production CLI

Rejected for v0 despite technical viability because it weakens the single-binary and environment-isolation goals.

### Go-only repository

Rejected. Python remains useful as a non-production oracle/tooling layer until Go conforms to every golden vector.

### Premature SQLite dependency

Rejected. JSONL remains canonical and a derived index must be fully rebuildable.

## Validation and reversibility

The decision is reversible before production implementation but requires new evidence of a material Go blocker. Adapter inconvenience or a small LOC difference alone is not a blocker.

The spike code is disposable evidence, not production code. Production implementation may use its tests/observations as requirements but should not copy the spike wholesale.
