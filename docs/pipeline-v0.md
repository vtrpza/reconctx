# Pipeline DAG and Approval Semantics v0

**Status:** implemented v0.1 contract; operator acceptance passed and publication remains pending
**Profile:** `web-blackbox`  
**Control mode:** operator-run artifact producer  
**Approved decisions:** `docs/product-decisions-v0.md`

## 1. Control boundary

```text
Operator starts CLI
  → CLI renders exact plan and waits
  → operator approves collection phase
  → CLI supervises approved GAU/Katana subprocesses
  → CLI normalizes and renders exact Arjun queue
  → operator approves, skips, or cancels Arjun phase
  → CLI supervises only approved Arjun subprocesses
  → CLI compiles a portable evidence handoff
  → operator gives the handoff to an agent
```

The agent is a later file consumer. The product exposes no MCP, daemon, runtime API, callback, or agent-callable active tool.

During product discovery, Hermes prepares and validates offline artifacts, but the operator executes every real scanner command, including loopback captures.

## 2. DAG

```text
                               OFFLINE / LOCAL-ONLY
Target + seeds + scope + wordlist + profile ──► validate inputs
                                      │
                                      ▼
                          resolve paths + versions
                                      │
                                      ▼
                            render effective plan
                                      │
                                      ▼
                   ┌──── APPROVAL A: COLLECTION ────┐
                   │                                │
                   ▼                                ▼
          GAU historical/passive            Katana active crawl
                   │                                │
                   └──────── raw capture ───────────┘
                                      │
                                      ▼
                      normalize + scope + correlate
                                      │
                                      ▼
                     deterministic candidate policy
                                      │
                                      ▼
                       arjun-candidates.jsonl
                                      │
                                      ▼
                    ┌── APPROVAL B: ARJUN QUEUE ──┐
                    │                             │
                 skip/cancel                    approve
                    │                             │
                    │                             ▼
                    │                 bounded Arjun executions
                    │                             │
                    └───────────────┬─────────────┘
                                    ▼
                     normalize + final correlation
                                    │
                                    ▼
                         compile handoff offline
                                    │
                                    ▼
                 validate schema, refs, hashes, checksums
                                    │
                                    ▼
                        terminal run status + package
```

Only the two tool branches in the diagram can perform target/provider network activity. Every step after raw capture is deterministic and offline.

## 3. Phase model

| Phase | Activity | Network | Approval | Output |
|---|---|---:|---|---|
| P0 input validation | offline | no | none | parsed target/scope/wordlist/profile |
| P1 preflight | local | no target traffic | none | paths, versions, writable workspace |
| P2 plan render | offline | no | none | immutable plan + digest |
| P3 initial collection | mixed | GAU provider + Katana target | Approval A | raw tool artifacts |
| P4 normalization/correlation | offline | no | none | schema v0 records |
| P5 candidate policy | offline | no | none | ranked included/excluded candidates |
| P6 parameter discovery | active | Arjun target | Approval B | per-target raw artifacts |
| P7 final normalization | offline | no | none | final records/relationships |
| P8 handoff compile | offline | no | none | `CONTEXT.md`, JSONL, manifest, raw selection |
| P9 integrity validation | offline | no | none | schema/ref/hash/checksum result |

`build` is offline and never inherits permission to start collection tools.

## 4. Run state machine

```text
planned
  ├─► preflight_failed
  └─► awaiting_collection_approval
          ├─► cancelled
          └─► collecting
                  ├─► interrupted
                  ├─► failed
                  └─► normalizing_initial
                          └─► awaiting_arjun_approval
                                  ├─► cancelled
                                  ├─► arjun_skipped
                                  └─► discovering_parameters
                                          ├─► interrupted
                                          └─► normalizing_final
                                                  └─► compiling
                                                          ├─► failed
                                                          ├─► partial
                                                          └─► success
```

Run status and tool status are independent. One failed ToolExecution can produce a `partial` run while valid observations from another execution remain available.

Intentional Arjun skip produces a collection-only handoff when initial collection completed. No Arjun ToolExecution is created; the phase/run coverage gap remains explicit and the handoff status is `partial`.

## 5. Plan artifact

The plan is data, not executable shell text. It contains:

```yaml
plan_version: reconctx-plan/v0
run_id: run_...
created_at: ...
inputs:
  target: ...
  seeds: [...]
  scope_path: scope.yaml
  scope_sha256: ...
  wordlist_path: /absolute/private/workspace/runs/run_.../inputs/wordlist.txt
  wordlist_sha256: sha256:...
  profile: web-blackbox
canonicalization_policy: url-canonicalization/v0
schema_version: reconctx/v0
tools:
  - name: gau
    resolved_path: /absolute/path
    version: 2.2.4
    activity_class: passive_external
    argv: [...]
    timeout_seconds: 45
    output_paths: [...]
  - name: katana
    resolved_path: /absolute/path
    version: v1.6.1
    activity_class: active_approved
    argv: [...]
    rate_limit_per_second: 2
    concurrency: 1
    parallelism: 1
    timeout_seconds: 10
limits:
  arjun_max_targets: 25
  arjun_request_budget: ...
environment_allowlist: [HOME, LANG, PATH, TZ]
environment: [HOME=/absolute/private/workspace/runs/run_.../home, LANG=C.UTF-8, PATH=/approved/bin:/usr/bin, TZ=UTC]
workspace_root: /absolute/no-symlink/path
plan_digest: sha256:...
```

Arguments are stored as arrays and executed without shell interpolation. A separate redacted display form is generated for logs/handoff.

`HOME` is bound to an empty private directory under the run, not inherited from the operator, so tools that require a home directory cannot load ambient user configuration.

Planning copies the bounded source wordlist byte-for-byte into that private run path before persisting the plan. The private copy, not the caller's source path, is hash-bound and used for later approval, resume, and Arjun execution.

`plan_digest` is SHA-256 over canonical JSON of all behavior-bearing fields. Cosmetic labels and terminal formatting do not enter the digest.

## 6. Approval A — initial collection

Before approval, the CLI terminal display includes:

- target and canonical seeds;
- scope path/hash, profile, wordlist path/hash, and canonicalization/schema policies;
- every tool path, metadata-derived version, and binary hash/mode/UID/GID/device/inode;
- activity class for each tool;
- exact effective argv in shell-escaped display form;
- per-tool rate, concurrency, parallelism, and timeout plus global Arjun ceilings/request budget;
- expected output paths;
- workspace path, environment allowlist, and exact effective environment values;
- plan digest.

Approval authorizes exactly the displayed digest. It does not authorize later commands or broader scope.

### Plan drift invalidates Approval A

A new approval is mandatory when any of these change:

- target, seed, scope rule, or scope digest;
- tool path or version;
- argv, environment allowlist, or effective environment value;
- activity classification;
- rate, concurrency, parallelism, or timeout;
- output/workspace path;
- enabled tool;
- canonicalization/schema version in behavior-bearing output.

A missing or unsupported tool fails planning; v0.1.0 does not silently produce a reduced plan.

## 7. Initial collection execution

GAU and each Katana seed execute sequentially in immutable plan order after Approval A.

### Isolation

Each ToolExecution receives a unique directory and occurrence ID:

```text
runs/<run-id>/executions/<tx-id>/
├── command.redacted.txt
├── version.txt
├── stdout.raw
├── stderr.raw
├── native-output.*
├── environment.safe.json
├── process-status.json
├── artifact-envelope.json
└── checksums.sha256
```

The runner never reuses an output path. This avoids GAU's append behavior and makes retry/resume auditable.

### Scope enforcement

- CLI filters/validates seeds before launch.
- Katana receives a tool-native crawl-scope constraint as defense in depth.
- Discovered out-of-scope/unknown URLs may be recorded but are never scheduled.
- GAU historical output may contain out-of-scope references; they remain record-only.
- Reconctx scope-checks every seed and generated Arjun candidate; tool-internal redirect behavior remains a documented residual risk.

### Capture

- stdout, stderr, and native output are captured independently under the applicable approved byte, record, and line limits; timestamps, exit code, resolved path, version, and redacted command are preserved separately;
- finalized captures are immutable after hashing; a capture-limit truncation is recorded on the artifact and forces partial execution/run coverage;
- parser failure never deletes the bounded captured bytes;
- target output is untrusted data.

## 8. Retry and timeout policy

v0 performs no automatic whole-tool retry by default.

Reasoning:

- active retries change target intensity;
- GAU output paths append and can duplicate records;
- Arjun retry can repeat brute-force traffic;
- a retry can cross the behavior approved by the operator.

A retry is an explicit new ToolExecution with a new output directory. If it changes intensity, scope, targets, argv, or candidate queue, it requires renewed approval.

Per-request retry behavior implemented internally by a source tool must be visible in the effective command/config or documented tool default.

Timeouts are phase/tool constraints, not proof that a target is absent. Timeout produces partial evidence and a diagnostic.

Conservative `web-blackbox` defaults:

| Tool | Limit |
|---|---|
| Katana | depth 2, 2 req/s, concurrency 1, parallelism 1, request timeout 10s |
| GAU | threads 1, execution/provider timeout 45s |
| Arjun | max 25 targets, 1 req/s, threads 1, request timeout 15s |

Increasing these values changes the plan digest and requires approval.

## 9. Cancellation and child cleanup

On first Ctrl-C:

1. atomically mark cancellation requested;
2. stop scheduling new candidates/processes;
3. send graceful termination to every child process group;
4. wait a bounded grace period;
5. force-kill remaining process groups;
6. close streams and flush manifests;
7. hash every artifact that can be read safely;
8. preserve parseable partial records;
9. mark affected executions and run `interrupted`;
10. do not auto-resume.

The runner launches children in isolated process groups and never through a shell. Completion requires a child-process leak check.

## 10. Normalization and correlation

Normalization consumes immutable artifact snapshots and emits schema v0 records.

Required order:

1. validate artifact hash/size;
2. parse the bounded in-memory artifact with format-specific validation;
3. preserve raw locator;
4. canonicalize URL under the recorded policy;
5. evaluate scope;
6. create Evidence;
7. create Observation;
8. correlate deterministic entities;
9. emit explicit diagnostics/gaps;
10. validate output records.

Historical, observed, inferred, bruteforced, and user-supplied states never merge semantically. Unknown GAU method is not converted to GET.

## 11. Arjun candidate policy

### Eligibility filter

Exclude:

- `out_of_scope` or `unknown` scope;
- static extension/MIME;
- fragment-only variants;
- canonical duplicates;
- excluded paths;
- historical-only endpoints unless explicitly opted in;
- methods/locations unsupported by the selected Arjun version;
- candidates beyond the approved ceiling.

### Deterministic ranking

Sort lexicographically by these descending booleans/weights, then by canonical Endpoint ID:

1. currently observed by Katana;
2. existing query-name evidence;
3. API-like path;
4. multiple independent source executions;
5. no static extension;
6. known/supported method and location.

No LLM participates. Every included and excluded candidate records reason codes and rank inputs.

### Candidate artifact

Each line includes:

- candidate/endpoint ID;
- raw selected URL and canonical route;
- proposed HTTP method and Arjun source mode;
- parameter location;
- source observations/evidence;
- scope decision;
- inclusion/exclusion reason codes;
- rank components;
- redacted public argv;
- estimated request budget;
- candidate policy version.

The selected queue is capped before Approval B. Excluded overflow remains visible.

## 12. Approval B — parameter discovery

The CLI terminal display includes:

- included count and target ceiling;
- exact ordered targets;
- URL, method, location, rank, and Evidence IDs;
- exact private wordlist and native-output paths;
- exact effective argv per target, including executable path, rate, threads, and timeout;
- per-candidate request budget;
- candidate queue digest.

The private immutable queue retains the exact execution bindings used by the digest. The portable candidate-decision artifact retains the wordlist hash, included and excluded policy decisions, rank inputs/reasons, scope decisions, source observations/executions, and a redacted argv that replaces the executable, wordlist, and native-output paths.

The operator may:

- approve the whole displayed queue;
- skip Arjun;
- cancel the run.

v0.1.0 does not edit or reorder the generated queue. Any candidate, limit, method/location, wordlist, request-budget, or effective-command change requires a new reviewed plan/run. Approval authorizes only the displayed queue digest.

## 13. Arjun execution

- only approved queue entries are scheduled;
- each target gets an independent ToolExecution or recoverable execution boundary;
- concurrency defaults to one target at a time in v0;
- target failure/timeout does not invalidate successful targets;
- absent native JSON plus explicit normal zero stdout is `success_zero`, not failure;
- detected parameters are candidates, not proof of completeness or vulnerability;
- method and parameter location remain explicit;
- no discovered parameter automatically schedules further activity.

## 14. Resume

Resume is operator-initiated and fail-closed. v0.1.0 supports only these persisted checkpoints:

- `planned` and `awaiting_collection_approval`: revalidate the immutable plan and require a fresh collection decision before network activity;
- `awaiting_arjun_approval`: revalidate the plan, scope, wordlist, workflow, immutable queue, and digest, then require a fresh Arjun decision;
- `compiling`: complete or verify the handoff offline;
- `success`: verify the existing handoff and report its path.

Persisted in-flight collection, normalization, or Arjun states are not retried. Failed, partial, interrupted, cancelled, and preflight-failed states are terminal for `resume`; they require a new reviewed plan. The separate offline `build` command may compile a valid persisted workflow from a supported compilable state.

## 15. Partial success matrix

| Condition | Tool status | Run behavior |
|---|---|---|
| GAU fails, Katana succeeds | GAU failed; Katana success | continue, handoff `partial`, declare historical gap |
| Katana fails, GAU succeeds | Katana failed; GAU success | continue normalization; default Arjun queue empty because historical-only; handoff `partial` |
| provider error + GAU exit 0 | semantic status partial/unknown | preserve records, provider diagnostics and coverage gap |
| unsupported native format | `unsupported_format` | preserve raw; do not invent observations; run partial if other facts exist |
| Arjun target timeout | target ToolExecution timed out | continue approved remaining targets; run partial |
| Arjun explicit zero | `success_zero`, coverage zero | continue; preserve stdout Evidence |
| operator skips Arjun | no Arjun ToolExecution | collection-only `partial` handoff with explicit parameter-discovery gap |
| cancellation | interrupted | stop children; preserve partial; compile only by a later explicit offline `build` when a valid workflow exists |
| no valid observations | failed or partial with zero facts | preserve manifest/raw; do not emit a misleading success handoff |
| handoff integrity failure | compile failed | retain workspace; package is not deliverable until rebuilt and verified |

## 16. Handoff compile and integrity gate

Compilation is offline and rebuildable. v0.1.0 raw policy:

- private workspace keeps the bounded captures; byte, record, or line limits may truncate them and mark execution/run coverage partial;
- the persisted workflow's raw sources are embedded byte-for-byte only after secret and private-path admission checks; a failed check blocks compilation rather than producing a derived redaction;
- the handoff manifest records `raw_policy: embedded_sanitized` for scan-admitted raw sources and `omitted` when the workflow requires no embedded raw bytes;
- v0 does not generate redacted raw derivatives and has no unredacted/full-raw inclusion option.

Delivery requires all of:

- every JSONL record validates under `reconctx/v0`;
- all record references resolve;
- every Evidence artifact/locator resolves or is explicitly withheld;
- manifest inventory size/SHA-256 matches files;
- package checksums pass;
- no managed path escapes the handoff root or traverses a symlink;
- secret scan policy passes;
- `CONTEXT.md` labels target content untrusted and lists gaps;
- no automatic finding/severity exists.

## 17. Approval record audit fields

Each approval record contains:

- approval phase;
- approved digest;
- private operator label, trimmed and currently limited to 1–128 bytes with Unicode control and format characters rejected;
- timestamp;
- decision: approve, skip, or cancel as permitted by the phase.

Approval records remain private run evidence. The handoff may expose redacted approval status/digest but not terminal identity or sensitive paths.

## 18. CLI implications

Implemented commands:

```bash
reconctx plan --target fixture.test --seed http://fixture.test:18080/ \
  --scope scope.yaml --wordlist /absolute/private/params.txt \
  --profile web-blackbox --workspace /absolute/private/reconctx-work \
  --out run-plan.json

reconctx run /absolute/private/reconctx-work/run-plan.json
# displays Approval A and later pauses for Approval B

reconctx resume --workspace /absolute/private/reconctx-work <run-id>
# continues only a supported approval/compile checkpoint

reconctx build --workspace /absolute/private/reconctx-work \
  --run <run-id> --out handoff/<run-id>
# offline only
```

v0 has no active `--yes` shortcut. Future non-interactive execution would require a separately designed signed approval artifact and is out of scope.

## 19. Gate closure criteria

Pipeline/approval gate is closed when:

- the two approval digests and invalidation rules are accepted;
- every network-producing node is downstream of an approval;
- candidate ranking and ceilings are deterministic;
- retry, timeout, cancellation, resume, and partial-success semantics are explicit;
- handoff build is offline and integrity-gated;
- no agent control plane exists;
- active discovery execution remains operator-controlled.

G1–G3 implementation is present and [G4 operator loopback acceptance](g4-acceptance-v0.1.0.md) passed. G5 artifact-specific publication approval remains open; this document does not authorize publication.
