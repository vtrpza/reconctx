# Pipeline DAG and Approval Semantics v0

**Status:** approved discovery contract  
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
  → operator approves, edits, skips, or cancels Arjun phase
  → CLI supervises only approved Arjun subprocesses
  → CLI compiles a portable evidence handoff
  → operator gives the handoff to an agent
```

The agent is a later file consumer. The product exposes no MCP, daemon, runtime API, callback, or agent-callable active tool.

During product discovery, Hermes prepares and validates offline artifacts, but the operator executes every real scanner command, including loopback captures.

## 2. DAG

```text
                               OFFLINE / LOCAL-ONLY
Target + seeds + scope + profile ──► validate inputs
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
                  skip/edit                     approve
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
| P0 input validation | offline | no | none | parsed target/scope/profile |
| P1 preflight | local | no target traffic | none | paths, versions, writable workspace |
| P2 plan render | offline | no | none | immutable plan + digest |
| P3 initial collection | mixed | GAU provider + Katana target | Approval A | raw tool artifacts |
| P4 normalization/correlation | offline | no | none | schema v0 records |
| P5 candidate policy | offline | no | none | ranked included/excluded candidates |
| P6 parameter discovery | active | Arjun target | Approval B | per-target raw artifacts |
| P7 final normalization | offline | no | none | final records/relationships |
| P8 handoff compile | offline | no | none | `CONTEXT.md`, JSONL, manifest, raw selection |
| P9 integrity validation | offline | no | none | schema/ref/hash/checksum result |

`build` and `ingest` are offline commands and never inherit permission to start collection tools.

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

Intentional Arjun skip produces a successful collection-only run when initial collection completed. The skipped ToolExecution/phase and resulting coverage gap remain explicit.

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
workspace_root: /absolute/no-symlink/path
plan_digest: sha256:...
```

Arguments are stored as arrays and executed without shell interpolation. A separate redacted display form is generated for logs/handoff.

`plan_digest` is SHA-256 over canonical JSON of all behavior-bearing fields. Cosmetic labels and terminal formatting do not enter the digest.

## 6. Approval A — initial collection

Before approval, the CLI displays:

- target, seeds, and canonical scope roots/exclusions;
- every tool path and observed version;
- activity class for each tool;
- exact effective argv in shell-escaped display form;
- rate, concurrency, parallelism, timeout, and retry policy;
- expected output paths;
- workspace path and raw policy;
- plan digest.

Approval authorizes exactly the displayed digest. It does not authorize later commands or broader scope.

### Plan drift invalidates Approval A

A new approval is mandatory when any of these change:

- target, seed, scope rule, or scope digest;
- tool path or version;
- argv or environment allowlist;
- activity classification;
- rate, concurrency, parallelism, timeout, or retry;
- output/workspace path;
- enabled tool;
- canonicalization/schema version in behavior-bearing output.

A missing tool may produce a reduced replacement plan, but the reduced plan receives a new digest and cannot reuse the previous approval.

## 7. Initial collection execution

GAU and Katana are independent branches and may run concurrently after Approval A.

### Isolation

Each ToolExecution receives a unique directory and occurrence ID:

```text
runs/<run-id>/executions/<tx-id>/
├── command.redacted.txt
├── version.txt
├── stdout.raw
├── stderr.raw
├── native/
├── environment.safe.json
├── process-status.json
└── checksums.sha256
```

The runner never reuses an output path. This avoids GAU's append behavior and makes retry/resume auditable.

### Scope enforcement

- CLI filters/validates seeds before launch.
- Katana receives a tool-native crawl-scope constraint as defense in depth.
- Discovered out-of-scope/unknown URLs may be recorded but are never scheduled.
- GAU historical output may contain out-of-scope references; they remain record-only.
- Redirect destinations are re-evaluated before scheduling any new request.

### Capture

- stdout, stderr, native output, timestamps, exit code, resolved path, version, and redacted command are preserved independently;
- raw files are append-only during the process and immutable after final hash;
- parser failure never deletes raw data;
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

A second Ctrl-C shortens the grace period but still attempts minimal process-status persistence.

The runner launches children in isolated process groups and never through a shell. Completion requires a child-process leak check.

## 10. Normalization and correlation

Normalization consumes immutable artifact snapshots and emits schema v0 records.

Required order:

1. validate artifact hash/size;
2. parse record with a bounded streaming parser;
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
- rendered argv;
- estimated request budget;
- candidate policy version.

The selected queue is capped before Approval B. Excluded overflow remains visible.

## 12. Approval B — parameter discovery

The CLI displays:

- included/excluded counts and ceiling;
- exact ordered targets;
- URL, method, body/location mode;
- source/ranking explanation;
- effective command per target;
- wordlist identity/hash;
- rate, threads, timeout;
- worst-case request estimate;
- candidate queue digest.

The operator may:

- approve the whole displayed queue;
- remove candidates;
- lower rates/threads/timeouts;
- skip Arjun;
- cancel the run.

Editing produces a new queue and digest. Approval authorizes only that digest.

Adding candidates, increasing limits, changing method/location/wordlist, or changing effective commands invalidates Approval B. Removing candidates or lowering intensity still produces a new digest and an explicit approval record; approval is never inferred.

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

Resume is operator-initiated and manifest-driven.

- verify existing artifact hashes first;
- never reopen finalized artifacts for append;
- keep completed ToolExecutions unchanged;
- create new IDs/directories for retried executions;
- rebuild normalized views deterministically from all selected execution records;
- re-render candidate policy when upstream observations change;
- invalidate Approval B if its queue digest changes;
- preserve earlier approval records and explain supersession.

Resume without new network activity—normalization or handoff rebuild—requires no approval. Any resumed network phase requires a still-valid matching approval or a new approval.

## 15. Partial success matrix

| Condition | Tool status | Run behavior |
|---|---|---|
| GAU fails, Katana succeeds | GAU failed; Katana success | continue, handoff `partial`, declare historical gap |
| Katana fails, GAU succeeds | Katana failed; GAU success | continue normalization; default Arjun queue empty because historical-only; handoff `partial` |
| provider error + GAU exit 0 | semantic status partial/unknown | preserve records, provider diagnostics and coverage gap |
| unsupported native format | `unsupported_format` | preserve raw; do not invent observations; run partial if other facts exist |
| Arjun target timeout | target ToolExecution timed out | continue approved remaining targets; run partial |
| Arjun explicit zero | `success_zero`, coverage zero | continue; preserve stdout Evidence |
| operator skips Arjun | skipped | successful collection-only handoff with explicit parameter-discovery gap |
| cancellation | interrupted | stop children; preserve partial; compile only by later explicit offline build/resume |
| no valid observations | failed or partial with zero facts | preserve manifest/raw; do not emit a misleading success handoff |
| handoff integrity failure | compile failed | retain workspace; package is not deliverable until rebuilt and verified |

## 16. Handoff compile and integrity gate

Compilation is offline and rebuildable. Default raw policy:

- private workspace keeps complete raw artifacts;
- handoff copies only evidence-referenced sanitized artifacts;
- omissions/references are explicit in manifest;
- full raw is opt-in.

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
- operator identity label (`operator`, not an external credential);
- timestamp;
- exact scope/profile/tool/candidate versions;
- decision: approve, edit, skip, cancel;
- superseded approval reference when applicable.

Approval records remain private run evidence. The handoff may expose redacted approval status/digest but not terminal identity or sensitive paths.

## 18. CLI implications

Conceptual commands:

```bash
reconctx plan --target fixture.test --scope scope.yaml \
  --profile web-blackbox --out run-plan.yaml

reconctx run run-plan.yaml
# displays Approval A and later pauses for Approval B

reconctx resume <run-id>
# validates manifests; asks only if network work remains

reconctx build --run <run-id> --profile compact --out handoff/<run-id>
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

This document satisfies those criteria for discovery v0. All discovery gates and the final implementation plan are complete; production code remains blocked only by explicit operator approval.
