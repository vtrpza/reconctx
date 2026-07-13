# Spike 001a — Python recon pipeline mechanics

## Question

Given a captured Katana JSONL fixture and a fake tool that spawns a child and ignores `SIGTERM`, can Python compile deterministic evidence artifacts and enforce bounded process-group cancellation using only the standard library?

## Workload

- parse the six-record Katana fixture;
- preserve raw URL and emit query-free route URL;
- write `events.jsonl` and `CONTEXT.md`;
- expose a runnable `compile` CLI;
- capture stdout/stderr from a fake subprocess;
- timeout, terminate, then kill the process group;
- verify no descendant remains.

## Evidence

```text
Python 3.13.5
Ran 3 tests in 0.391s
OK
CLI summary: {"records":6,"unique_routes":6}
500 compile iterations:
  median_ns_per_op=110686
  mean_ns_per_op=112840
Fake-process leaks: 0
Source: 92 lines / 3178 bytes
SQLite stdlib: 3.46.1
External runtime dependencies: 0
```

The generated six events were semantically equal to the Go output, and `CONTEXT.md` was byte-identical.

## Verdict: VALIDATED

### What worked

- Fast adapter implementation with compact code.
- Standard-library JSON, URL, subprocess groups, tests, and SQLite.
- Correct timeout escalation and preservation of both streams.
- Straightforward CLI and artifact generation.

### What did not resolve the production decision

- Distribution still requires a compatible Python runtime or a separate bundling system.
- Python environment inheritance is a real integration risk for Python-based tools; Arjun already required clearing `PYTHONPATH`.
- Static type guarantees are weaker unless additional tooling/dependencies are introduced.

### Recommendation

Keep Python for executable specifications, fixtures, discovery scripts, and benchmark tooling. It is technically viable for the production CLI but does not overturn the approved Go default.
