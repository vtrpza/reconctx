# Spike 001b — Go recon pipeline mechanics

## Question

Given the same captured Katana JSONL fixture and fake hostile process tree, can Go compile equivalent artifacts and enforce bounded process-group cancellation using only the standard library while producing a distributable binary?

## Workload

Identical behavioral contract to spike 001a:

- six-record JSONL parse;
- raw URL + query-free route URL;
- `events.jsonl` + byte-equivalent `CONTEXT.md`;
- runnable `compile` CLI;
- stdout/stderr capture;
- timeout, process-group TERM/KILL escalation;
- descendant leak check.

## Evidence

```text
Go 1.24.4 linux/amd64
3 tests passed in 0.354s
CLI summary: {"records":6,"unique_routes":6}
BenchmarkCompileFixture median of three runs:
  76128 ns/op
  72035-72058 B/op
  129 allocs/op
Fake-process leaks: 0
Stripped binary: 1994936 bytes (1.903 MiB)
Source: 193 lines / 4689 bytes
Standard-library dependency packages in build graph: 69
External Go modules: 0
```

The generated six events were semantically equal to the Python output, and `CONTEXT.md` was byte-identical.

## Verdict: VALIDATED

### What worked

- Single stripped binary with no external module dependency.
- Explicit process-group lifecycle and bounded cancellation.
- Streaming scanner, typed records, deterministic JSONL, and CLI packaging.
- Approximately 31.2% lower median latency than Python in this tiny file-heavy microbenchmark; this is directional only, not a product KPI.

### Costs and open points

- The spike used about 2.10x the source lines of the Python implementation.
- Full UTS-46/IDNA, Draft 2020-12 JSON Schema validation, and optional SQLite would likely add reviewed dependencies.
- Linux process hardening still needs production integration tests for symlinks, grandchildren, cancellation races, and output limits.

### Recommendation

Use Go for the production CLI. The exercised subprocess, streaming, artifact, and distribution paths showed no material blocker.
