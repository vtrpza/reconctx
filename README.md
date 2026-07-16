# reconctx

> Evidence, not terminal noise.

`reconctx` turns bounded GAU, Katana, and Arjun collection into a portable, checksummed handoff for human or agent analysis. Native artifacts stay traceable, normalized facts keep their provenance, and incomplete coverage stays incomplete.

The operator owns every network decision. A downstream agent receives files—not scanner control.

**Release candidate:** v0.1.0 for Linux amd64 · GAU 2.2.4 · Katana v1.6.1 · Arjun 2.2.7 · Apache-2.0

G4 operator acceptance passed. No public tag or release exists yet; the final artifact set and independent review remain pending.

[Inspect a complete sanitized handoff](examples/handoff-web-blackbox-v0/CONTEXT.md) · [CLI contract](docs/cli.md) · [Threat model](docs/threat-model.md)

## See the result before running anything

```bash
(cd examples/handoff-web-blackbox-v0 && sha256sum -c checksums.sha256)
sed -n '1,160p' examples/handoff-web-blackbox-v0/CONTEXT.md
```

The example is deterministic and generated only from sanitized fixtures. `CONTEXT.md` is the factual front door; JSONL projections support machines and agents; Evidence locators, a manifest, and checksums support independent verification.

```text
handoff/<run-id>/
├── CONTEXT.md                 # compact, evidence-linked surface
├── manifest.json
├── checksums.sha256
├── normalized/
│   ├── records.jsonl          # complete normalized record stream
│   ├── agent-view.jsonl       # deterministic compact projection
│   └── *.jsonl                # typed views and Evidence index
└── raw/                       # optional admitted native bytes; review before sharing
```

The handoff is readable without `reconctx` or the scanners installed.

## Control flow

```text
plan (offline)
  → approve exact collection digest
  → bounded GAU provider query + bounded Katana crawl
  → normalize, scope, correlate, and rank offline
  → decide on the exact Arjun queue digest
      ├ approve → bounded Arjun execution → compile offline
      ├ skip    → compile partial with an explicit gap
      └ cancel  → stop
```

| Command | Scanner or provider traffic |
|---|---|
| `plan` | None. Validates inputs and reads bounded tool metadata without starting a scanner. |
| `run` | Only after an operator at a terminal enters the full displayed digest. |
| `resume` | Only from supported checkpoints, with a fresh approval when activity can resume. |
| `build` | None. Reads persisted evidence and never resolves or launches a scanner. |

There is no `--yes`, agent approval path, daemon, MCP server, or runtime callback. Drift in the digest-bound plan or queue, or in a revalidated tool identity, invalidates the decision.

## Run the owned loopback demo

Requirements: Linux amd64, Go 1.24+, and the exact scanner versions listed above. Scanner binaries are not bundled and must be trusted by the operator. A source build reports `0.0.0-dev`; release binaries receive their version at build time.

Even this loopback run can contact GAU's pinned external providers. Review the rendered plan and approve only the traffic you intend to authorize.

Terminal 1:

```bash
python3 -m fixture_target.app --host 127.0.0.1 --port 18080
```

Terminal 2:

```bash
go build -o ./reconctx ./cmd/reconctx
WORKSPACE="$(mktemp -d /tmp/reconctx-demo.XXXXXX)"

./reconctx plan \
  --target 127.0.0.1 \
  --seed http://127.0.0.1:18080/ \
  --scope examples/scope.yaml \
  --wordlist fixtures/shared/arjun-minimal.txt \
  --profile web-blackbox \
  --workspace "$WORKSPACE" \
  --out "$WORKSPACE/plan.json"

./reconctx run --out handoff/demo "$WORKSPACE/plan.json"

(cd "$WORKSPACE/handoff/demo" && sha256sum -c checksums.sha256)
sed -n '1,180p' "$WORKSPACE/handoff/demo/CONTEXT.md"
```

At each approval gate, inspect the displayed scope, tool identities, hashes, arguments, environment, limits, queue, and output paths. Enter the exact decision plus the full digest, then the private operator label. Stop the fixture with `Ctrl+C` after the run.

`run` already compiles the handoff. `resume` continues only supported approval/compile checkpoints or verifies a successful handoff; custom output paths must be repeated. `build` is an offline rebuild and requires a new output path:

```bash
./reconctx resume \
  --workspace "$WORKSPACE" \
  --out handoff/demo \
  RUN_ID

./reconctx build \
  --workspace "$WORKSPACE" \
  --run RUN_ID \
  --out handoff/verify-RUN_ID
```

For authorized non-fixture targets, create a matching strict scope document and use an absolute operator-owned private workspace. Read the [complete CLI contract](docs/cli.md) before active use.

## Evidence semantics

Every evidence-bearing observation resolves to an Evidence ID, an artifact SHA-256, and a native locator such as a line range or JSON Pointer.

`reconctx` deliberately keeps these meanings separate:

- a GAU URL is historical, not proof of current reachability;
- an HTTP response was observed only during the recorded run;
- an Arjun parameter is a candidate, not a finding;
- a bounded zero result is not proof of universal absence;
- process completion and semantic coverage are different states;
- timeout, interruption, truncation, unsupported output, and operator skip remain explicit gaps.

Raw occurrences are preserved even when canonical entities deduplicate them. Exit code `0` alone never establishes complete coverage.

## Security boundaries

- Strict scope and fixed rate, concurrency, parallelism, timeout, and target ceilings constrain active scheduling.
- Collection and Arjun decisions bind behavior-bearing state to full SHA-256 digests.
- Preflight binds supported tool versions, executable SHA-256, filesystem identity, paths, and effective environment; drift fails closed.
- Subprocesses receive argument vectors without shell interpolation, a private approved `HOME`, bounded captures, and process-group containment.
- Target and tool output is untrusted data, never instructions.
- `reconctx` assumes trusted tools; it is not a sandbox for a hostile scanner binary. URL scope also cannot eliminate DNS rebinding or unsafe internal scanner behavior.
- Admitted raw bytes may be copied into the handoff. Secret scanning is heuristic, so a human must review every handoff before sharing it.
- No disk quota or free-space preflight exists; operators must choose and monitor workspace capacity.

See [SECURITY.md](SECURITY.md), the [scope contract](docs/scope-v0.md), the [compatibility matrix](docs/compatibility-matrix-v0.md), and the [security test matrix](docs/security-test-matrix.md).

## Proof, not promise

The accepted v0.1.0 loopback run ended `partial`—correctly preserving GAU, Katana, and Arjun coverage gaps instead of claiming completeness. Independent validation accepted all 62 schema-valid records and resolved every record, artifact, and Evidence reference. All 32 checksum entries passed, and two additional offline builds were byte-identical to the original handoff. The exact candidate, tool hashes, approval digests, decisions, and results are in the [G4 acceptance record](docs/g4-acceptance-v0.1.0.md).

In the repository fixture benchmark, one 7,064-byte `CONTEXT.md` answered all 10 protocol questions without drill-down, using 89.8% fewer source bytes than the normalized stream; all 15 cited Evidence IDs resolved. This measures that fixture and protocol, not scanner coverage or model quality. See the [agent handoff benchmark](benchmarks/agent-handoff-v1.md).

## Deliberate boundaries

v0.1.0 supports Linux amd64 and only the pinned tool contracts above. It does not perform exploitation, generate findings or severity, ingest authenticated HAR/Burp data, import BBOT, run autonomously, expose a dashboard, or distribute execution.

Arjun's private Python environment must remain immutable between planning and execution; v0.1.0 does not hash the entire environment closure. Supporting another platform or scanner version requires captured behavior, sanitized fixtures, adapter and failure-path tests, and renewed acceptance where process semantics change.

## Develop and verify

```bash
python3 -m venv .tools/schema-venv
.tools/schema-venv/bin/python -m pip install \
  --require-hashes --requirement requirements-dev.lock

go test ./...
go test -race ./...
go vet ./...
.tools/schema-venv/bin/python -m unittest discover -s tests -v
(cd examples/handoff-web-blackbox-v0 && sha256sum -c checksums.sha256)
```

CI performs the core checks, validates fixture/reference determinism, and compares two static Linux amd64 builds. CI and normal tests never launch real scanners. Contribution rules and fixture hygiene are in [CONTRIBUTING.md](CONTRIBUTING.md).

Licensed under the [Apache License 2.0](LICENSE).
