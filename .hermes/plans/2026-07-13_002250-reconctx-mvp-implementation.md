# reconctx MVP Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Build the Linux-first `reconctx` production CLI that lets an operator approve and supervise GAU, Katana and Arjun, then compiles a portable evidence-first handoff without exposing active execution to an agent.

**Architecture:** A dependency-light Go CLI implements the state machine, approval digests, safe workspace, process supervision, adapters, deterministic candidate policy and handoff compiler. JSONL remains canonical; Python remains a test oracle for schemas/canonicalization/fixtures and is not a runtime dependency. All scanner execution stays behind explicit operator approvals; development and CI use captured fixtures and fake subprocesses only.

**Tech Stack:** Go production binary; Go standard library by default; `golang.org/x/net/idna` for UTS-46 after dependency review; `golang.org/x/sys/unix` for Linux filesystem/process primitives; Python 3.13 discovery oracle; Draft 2020-12 public schemas; JSON/JSONL canonical artifacts; Apache-2.0.

**Approval:** implementation and publication explicitly approved by the operator at `2026-07-13T12:50:05-03:00`. Safety/review gates G1–G5 remain mandatory execution gates.

---

## 0. Binding decisions and stop gates

This plan does **not** authorize implementation, scanner execution, GitHub publication or repository creation. Production work starts only after the operator explicitly approves this plan.

### Architecture decisions resolved here

1. **Production language:** Go.
2. **Canonical store:** JSONL/files; no SQLite in MVP.
3. **Schema strategy:** typed Go models plus explicit semantic validators at runtime; Python `jsonschema` remains the CI/golden oracle against `schemas/v0/`.
4. **CLI:** standard-library subcommands (`flag.FlagSet`); avoid Cobra unless usability evidence justifies it.
5. **YAML:** one reviewed, pinned YAML dependency only for `scope.yaml`/profile input; normalized internal representation is canonical JSON.
6. **IDNA:** reviewed/pinned `golang.org/x/net/idna`; all existing compatibility vectors must pass before use in scope decisions.
7. **Linux workspace safety:** trusted directory handle plus no-follow/open-beneath operations where supported; fail closed when the required invariant cannot be established.
8. **External tools:** direct argv execution only; no shell; absolute path/version/file identity rechecked immediately before launch.
9. **Agent boundary:** file consumer only; no MCP, API, daemon or agent-callable scanner.
10. **Publication:** local repository first; no GitHub remote until a separate explicit publication approval.

### STOP gates

| Gate | Must be true before proceeding | Prohibited before pass |
|---|---|---|
| G0 — implementation approval | Operator explicitly approves this exact plan | Any production Go implementation or repo publication |
| G1 — offline core safety | Canonicalization, scope, path safety, digest and state-machine tests pass | Any external tool process, including loopback scanner |
| G2 — fake-runner safety | Fake subprocess timeout/cancel/tree cleanup, artifact immutability and bounds pass | Wiring GAU/Katana/Arjun execution |
| G3 — adapter/compiler conformance | All captured normal/failure fixtures parse conservatively; handoff validates and rebuilds deterministically | Operator acceptance run |
| G4 — operator acceptance | Operator reviews exact plan and runs bounded loopback commands; schemas/checksums/leak checks pass | Release candidate/publication |
| G5 — publication approval | README/security/fixtures/license/release artifacts reviewed and operator explicitly approves publication | Creating/pushing GitHub repository or release |

If a gate fails, stop, preserve evidence and update the plan/ADR. Do not weaken a control to make a test pass.

---

## 1. Target repository layout

Implementation extends the current discovery workspace without copying spike code wholesale:

```text
cmd/reconctx/main.go
internal/
├── app/                 # orchestration and dependency wiring
├── approval/            # Approval A/B records and digest binding
├── artifact/            # immutable captures, hashes, locators
├── adapter/
│   ├── gau/
│   ├── katana/
│   └── arjun/
├── candidate/           # deterministic Arjun queue
├── canonical/           # canonical JSON and URL policy
├── cli/                 # plan/run/resume/build subcommands
├── compiler/            # records, CONTEXT, manifest, checksums
├── integrity/           # schema/ref/hash/secret/path gates
├── model/               # typed domain/state records
├── preflight/           # tool resolution/version/env policy
├── process/             # direct exec, groups, streams, cancellation
├── scope/               # scope.yaml and decisions
└── workspace/           # safe rooted filesystem operations
profiles/web-blackbox.yaml
schemas/v0/              # existing public contracts
fixtures/cases/          # existing sanitized oracle corpus
reference/               # existing Python executable specification
integration/faketools/   # network-inert fake binaries/scripts
integration/testdata/    # plans/scopes/golden package expectations
tests/                   # existing Python cross-conformance tests
```

Public CLI:

```bash
reconctx plan --target fixture.test --scope scope.yaml \
  --profile web-blackbox --workspace ./work --out run-plan.json

reconctx run run-plan.json
# interactive Approval A; later interactive Approval B

reconctx resume <run-id>
reconctx build --run <run-id> --profile compact --out handoff/<run-id>
```

There is no active `--yes` in v0. `build` and normalization paths are offline-only.

---

## 2. Task sequence

### Task 1: Bootstrap the local Go module after G0

**Objective:** Establish a testable local CLI skeleton without a remote and without scanner behavior.

**Files:**
- Create: `go.mod`
- Create: `cmd/reconctx/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/root_test.go`
- Create: `internal/version/version.go`
- Modify: `.gitignore`

**Steps:**

1. Write `internal/cli/root_test.go` first. Assert `help`, unknown-command exit `2`, and no network/process creation.
2. Run `go test ./internal/cli -run TestRoot -v`; expect RED because the package does not exist.
3. Initialize `go.mod` as `github.com/vtrpza/reconctx` and implement only subcommand dispatch/help.
4. Run the focused test; expect GREEN.
5. Run `go test ./...` and the existing Python suite.
6. Optionally initialize local Git only after G0; do not add a remote.

**Verification:**

```bash
go test ./...
.tools/schema-venv/bin/python -m unittest discover -s tests -q
```

**Commit after approval:** `chore: bootstrap reconctx Go CLI`

---

### Task 2: Define typed model, canonical JSON and behavior digests

**Objective:** Make plan/approval/run semantics deterministic before any runner exists.

**Files:**
- Create: `internal/model/plan.go`
- Create: `internal/model/run.go`
- Create: `internal/model/approval.go`
- Create: `internal/canonical/json.go`
- Create: `internal/canonical/json_test.go`
- Create: `internal/approval/digest.go`
- Create: `internal/approval/digest_test.go`

**Required types:**

```go
type RunState string

const (
    RunPlanned                    RunState = "planned"
    RunAwaitingCollectionApproval RunState = "awaiting_collection_approval"
    RunCollecting                 RunState = "collecting"
    RunAwaitingArjunApproval      RunState = "awaiting_arjun_approval"
    RunCompiling                  RunState = "compiling"
    RunPartial                    RunState = "partial"
    RunSuccess                    RunState = "success"
    RunFailed                     RunState = "failed"
    RunInterrupted                RunState = "interrupted"
)

type ApprovalRecord struct {
    Phase            string `json:"phase"`
    ApprovedDigest   string `json:"approved_digest"`
    OperatorLabel    string `json:"operator_label"`
    Decision         string `json:"decision"`
    Supersedes       string `json:"supersedes,omitempty"`
    CreatedAt        string `json:"created_at"`
}
```

**TDD cases:**

1. Same behavior-bearing plan in different map order → same digest.
2. Display labels/terminal formatting → no digest change.
3. Scope, path, version, argv, limits, environment allowlist or output path change → digest changes.
4. NaN/duplicate keys/unsupported values → fail closed.
5. Approval digest mismatch → transition denied before child creation.

**Verification:** `go test ./internal/canonical ./internal/approval -v`

**Commit:** `feat: add canonical plan and approval digests`

---

### Task 3: Port URL canonicalization and scope evaluation

**Objective:** Match `url-canonicalization/v0` before any target can be scheduled.

**Files:**
- Create: `internal/canonical/url.go`
- Create: `internal/canonical/url_test.go`
- Create: `internal/scope/model.go`
- Create: `internal/scope/load.go`
- Create: `internal/scope/evaluate.go`
- Create: `internal/scope/evaluate_test.go`
- Read oracle: `reference/canonicalization_v0.py`
- Read vectors: `schemas/v0/canonicalization-compatibility-v0.json`

**TDD order:**

1. Load every existing compatibility vector in a Go table test; verify RED.
2. Implement scheme/host/port/path/query normalization incrementally.
3. Add UTS-46/IDNA only after license/version review; pin dependency.
4. Reject userinfo, backslashes, malformed percent encodings, unsupported schemes, non-standard numeric IP forms and unknown scope.
5. Prove planner scope and candidate scope call the same evaluator.
6. Differentially compare Go output to the Python oracle in CI.

**Verification:**

```bash
go test ./internal/canonical ./internal/scope -v
.tools/schema-venv/bin/python -m unittest tests.test_canonicalization_v0 -v
```

**STOP G1 condition:** every vector passes identically; no “close enough” canonicalization.

**Commit:** `feat: implement canonical URL and scope policy v0`

---

### Task 4: Implement rooted workspace safety

**Objective:** Ensure all managed reads/writes remain under a trusted workspace without symlink/special-file abuse.

**Files:**
- Create: `internal/workspace/root_linux.go`
- Create: `internal/workspace/root_other.go`
- Create: `internal/workspace/root_test.go`
- Create: `internal/workspace/atomic.go`
- Create: `internal/workspace/atomic_test.go`

**TDD cases:**

1. Reject absolute managed relative paths and `..`.
2. Reject symlink at every path segment.
3. Reject FIFO, socket, device and unsafe hardlink conditions where detectable.
4. Create private run directories as `0700` and evidence files as `0600`.
5. Use unique execution directories; refuse append/overwrite of finalized artifacts.
6. Atomic temp-write + fsync + rename for metadata/JSONL views.
7. Hold/revalidate trusted directory identity; fail closed if the root changes.
8. Hand off Linux-specific `openat2`/no-follow behavior behind a small interface.

**Verification:** `go test ./internal/workspace -v`

**Commit:** `feat: add safe rooted workspace operations`

---

### Task 5: Build preflight and immutable plan rendering

**Objective:** Resolve exact tools/config without target traffic and render Approval A data.

**Files:**
- Create: `internal/preflight/tool.go`
- Create: `internal/preflight/version.go`
- Create: `internal/preflight/environment.go`
- Create: `internal/preflight/preflight_test.go`
- Create: `internal/app/plan.go`
- Create: `internal/app/plan_test.go`
- Create: `profiles/web-blackbox.yaml`

**Required behavior:**

- Resolve absolute realpaths and record file identity/hash/permissions.
- Reject unexpected writable parent ownership/permissions per documented policy.
- Clear dangerous loader/proxy/Python variables unless explicitly modeled.
- Arjun version parsing must reject `127.0.0` from help and accept runtime/package `2.2.7`.
- Render exact argv arrays, redacted display, limits, paths, activity classes and plan digest.
- No tool subprocess with network-producing argv runs during `plan`; version probes use fake tools in tests.
- Tool missing/unsupported produces preflight failure or a newly digested reduced plan—never approval reuse.

**TDD fixtures:** fake `gau`, `katana`, `arjun` binaries under `integration/faketools/`; no network APIs.

**Verification:** `go test ./internal/preflight ./internal/app -run 'TestPlan|TestPreflight' -v`

**Commit:** `feat: render immutable preflighted collection plans`

---

### Task 6: Implement approval state machine

**Objective:** Make it impossible to start a network-producing process without a matching current digest.

**Files:**
- Create: `internal/approval/state.go`
- Create: `internal/approval/state_test.go`
- Create: `internal/app/state_machine.go`
- Create: `internal/app/state_machine_test.go`

**TDD cases:**

1. Planned → awaiting Approval A without process creation.
2. Exact approval → collecting.
3. Any plan drift → approval invalidated.
4. Candidate queue generation → awaiting Approval B.
5. Edit/remove/lower limits → new queue digest and explicit approval required.
6. Add/increase/change method/location/wordlist → old approval rejected.
7. Skip Arjun → successful collection-only path with explicit gap.
8. Cancel → no new scheduling.
9. No active `--yes` or implicit approval path exists.
10. Approval records are append-only and preserve supersession.

**Verification:** `go test ./internal/approval ./internal/app -run 'TestApproval|TestTransition' -v`

**STOP G1:** state/digest/scope/workspace suites all green before process supervisor work.

**Commit:** `feat: enforce two-phase approval state machine`

---

### Task 7: Implement the fake-only process supervisor and artifact capture

**Objective:** Prove process safety and evidence preservation without running recon tools.

**Files:**
- Create: `internal/process/spec.go`
- Create: `internal/process/supervisor_linux.go`
- Create: `internal/process/supervisor_test.go`
- Create: `internal/artifact/capture.go`
- Create: `internal/artifact/capture_test.go`
- Reuse behavior: `spikes/fake_tool.py`

**Required behavior:**

- Direct `exec.Cmd` argv; never shell.
- Dedicated child process group.
- Concurrent bounded stdout/stderr capture to private files; no raw terminal replay.
- Context timeout/cancel: stop scheduling, TERM process group, bounded wait, CONT stopped children if needed, KILL fallback, reap, flush and hash.
- Preserve exit/signal/timing, resolved binary identity, argv display, environment allowlist and native outputs independently.
- Enforce per-file/line/record limits; mark truncation/partial diagnostics.
- Finalized artifacts immutable; retries use new execution IDs/directories.
- No descendant leaks.

**Regression derived from discovery:** a non-interactive background child may ignore `SIGINT`; cleanup must not wait unboundedly on `SIGINT`.

**TDD cases:** normal, exit non-zero with partial output, timeout, child ignores TERM, stopped child, grandchild, invalid UTF-8, ANSI/OSC/CR, oversized output, second cancellation, disk failure.

**Verification:**

```bash
go test ./internal/process ./internal/artifact -v
pgrep -af 'integration/faketools|fake_tool.py'  # expected no matches
```

**STOP G2:** all fake process-tree/leak/artifact tests pass before adapters can invoke real tool paths.

**Commit:** `feat: supervise bounded process trees and immutable artifacts`

---

### Task 8: Implement fixture-only adapters

**Objective:** Parse captured outputs conservatively into schema v0 records without executing tools.

**Files:**
- Create: `internal/adapter/adapter.go`
- Create: `internal/adapter/gau/parser.go`
- Create: `internal/adapter/gau/parser_test.go`
- Create: `internal/adapter/katana/parser.go`
- Create: `internal/adapter/katana/parser_test.go`
- Create: `internal/adapter/arjun/parser.go`
- Create: `internal/adapter/arjun/parser_test.go`
- Create: `internal/model/records.go`
- Create: `internal/model/validate.go`

**Golden cases:** all directories under `fixtures/cases/`, including:

- `KAT-NORMAL-MINIMAL`
- `KAT-INTERRUPTED-LOOPBACK`
- `ARJUN-GET-FOUND`, `ARJUN-POST-FORM-FOUND`, `ARJUN-JSON-FOUND`, `ARJUN-ZERO`
- `ARJUN-INTERRUPTED-LOOPBACK`
- `ARJUN-REQUEST-TIMEOUT-LOOPBACK`
- both GAU normal/regression cases

**Mandatory semantic assertions:**

- GAU historical does not imply current reachability or GET.
- Katana parses valid JSONL siblings independently; interruption is partial coverage.
- Arjun zero = exit 0 + explicit zero stdout + absent native JSON.
- Arjun interruption = exit 124 + absent native JSON → unknown result, not zero.
- Arjun timeout fixture = exit 1 + traceback → failed/tool-error, no absence claim.
- Unknown format preserves raw and emits `unsupported_format`.
- Every fact has execution/artifact/line-or-pointer Evidence.

**Verification:**

```bash
go test ./internal/adapter/... -v
.tools/schema-venv/bin/python -m unittest tests.test_failure_fixtures -v
```

**Commit:** `feat: add evidence-preserving GAU Katana and Arjun adapters`

---

### Task 9: Implement deterministic Arjun candidate policy and Approval B artifact

**Objective:** Produce an explainable, bounded queue with no LLM participation.

**Files:**
- Create: `internal/candidate/policy.go`
- Create: `internal/candidate/policy_test.go`
- Create: `internal/candidate/digest.go`
- Create: `internal/candidate/digest_test.go`

**Tests first:** eligibility exclusions, static extension, fragment-only, canonical duplicates, excluded path, historical-only default, unsupported method/location and max-target overflow.

**Ranking order:** observed by Katana; query evidence; API-like path; multi-source; non-static; supported method/location; final tie-break by canonical Endpoint ID.

**Every row includes:** included/excluded state, reason codes, rank components, Evidence IDs, proposed method/location, effective argv, request estimate, policy version.

**Approval B tests:** exact digest binding; editing always creates a new digest; queue cannot exceed plan ceiling; only approved entries reach scheduling.

**Verification:** `go test ./internal/candidate ./internal/approval -v`

**Commit:** `feat: compile deterministic approved Arjun queues`

---

### Task 10: Implement handoff compiler and integrity gate

**Objective:** Rebuild the benchmarked portable package from normalized records.

**Files:**
- Create: `internal/compiler/context.go`
- Create: `internal/compiler/agent_view.go`
- Create: `internal/compiler/package.go`
- Create: `internal/compiler/compiler_test.go`
- Create: `internal/integrity/references.go`
- Create: `internal/integrity/checksums.go`
- Create: `internal/integrity/secrets.go`
- Create: `internal/integrity/integrity_test.go`

**Required outputs:** `README.md`, `CONTEXT.md`, `manifest.json`, `checksums.sha256`, canonical/split JSONL, `agent-view.jsonl`, `arjun-candidates.jsonl`, and selected sanitized evidence artifacts.

**Tests:**

1. Go compiler matches canonical record IDs/semantic counts from `examples/handoff-web-blackbox-v0/`.
2. Rebuild twice → byte-identical output.
3. All schemas/references/Evidence locators resolve.
4. Inventory sizes/SHA-256 and package checksums pass.
5. Secret/path/symlink/special-file gate fails closed.
6. Target text is labeled untrusted and cannot enter instruction sections.
7. No finding/severity record or unsupported claim is emitted.
8. Compact front door remains smaller than selected raw while retaining all Evidence IDs.
9. Private raw remains private; copied/referenced/omitted/redacted material is declared.

**Verification:**

```bash
go test ./internal/compiler ./internal/integrity -v
.tools/schema-venv/bin/python -m unittest tests.test_example_v0 tests.test_schema_v0 -v
```

**STOP G3:** adapters/compiler/integrity pass all normal and failure fixtures before operator acceptance.

**Commit:** `feat: compile portable integrity-gated handoffs`

---

### Task 11: Wire `plan`, `run`, `resume` and `build`

**Objective:** Integrate the state machine while preserving offline/active boundaries.

**Files:**
- Create: `internal/cli/plan.go`
- Create: `internal/cli/run.go`
- Create: `internal/cli/resume.go`
- Create: `internal/cli/build.go`
- Create: `internal/cli/integration_test.go`
- Modify: `internal/app/*`

**Integration order:**

1. `plan`: input/preflight/render/digest only.
2. `build`: offline compile only; must not import process runner.
3. `resume`: hash/state validation; offline rebuild without approval; network continuation only with valid/new approval.
4. `run`: Approval A, collection, normalization, queue, Approval B, optional Arjun, final compile.

**Fake-tool E2E scenarios:** success, GAU fail/Katana success, Katana fail/GAU success, Arjun skip, Arjun zero, Arjun target error, interrupt, resume hash mismatch and integrity failure.

**Architectural test:** inject a process-launch recorder and assert zero launches while awaiting either approval and throughout `build`.

**Verification:** `go test ./internal/cli ./internal/app -v`

**Commit:** `feat: wire operator-controlled recon workflow`

---

### Task 12: Cross-conformance, fuzzing and security gates

**Objective:** Prove the Go implementation against the discovery oracle and threat model.

**Files:**
- Create: `integration/conformance_test.go`
- Create: `internal/canonical/url_fuzz_test.go`
- Create: `internal/adapter/katana/fuzz_test.go`
- Create: `internal/integrity/path_fuzz_test.go`
- Create: `docs/security-test-matrix.md`

**Required checks:**

```bash
go test ./...
go test -race ./...
go vet ./...
go test ./internal/canonical ./internal/adapter/... ./internal/integrity -fuzz=Fuzz -fuzztime=30s
.tools/schema-venv/bin/python -m unittest discover -s tests -q
```

Map every T-01…T-15 threat-model class to at least one automated test or explicit manual/release gate. Record dynamic limitations rather than claiming universal safety.

**Commit:** `test: add cross-conformance and security gates`

---

### Task 13: Operator loopback acceptance after G3

**Objective:** Demonstrate the production CLI under operator control without authorizing external targets.

**Owner:** operator executes all real GAU/Katana/Arjun commands. Hermes may prepare previews and perform offline validation afterward.

**Sequence:**

1. Hermes renders exact loopback plan and both approvals without executing.
2. Operator reviews path/version/hash/scope/argv/limits/output roots.
3. Operator approves and runs bounded initial collection.
4. Hermes validates private captures and renders exact Arjun queue offline.
5. Operator approves/edits/skips and executes Arjun.
6. Hermes validates final handoff offline: schemas, references, hashes, checksums, paths, secrets, process leaks.
7. Repeat interruption once only if the new production runner needs evidence beyond existing fixtures.

**G4 pass criteria:** no unapproved child, no scope escape, no leak, correct partial semantics, deterministic rebuild, valid handoff, benchmark questions answerable with valid Evidence IDs.

No external target is authorized by this plan.

---

### Task 14: Documentation, local release candidate and publication gate

**Objective:** Produce a reviewable local artifact without publishing it.

**Files:**
- Modify: `README.md`
- Modify: `SECURITY.md`
- Modify: `CONTRIBUTING.md`
- Modify: `docs/compatibility-matrix-v0.md`
- Create: `docs/cli.md`
- Create: `docs/release-checklist.md`
- Create: `CHANGELOG.md`

**Release artifacts:** Linux amd64 binary, SHA-256 checksum, SBOM/dependency/license inventory, test log and dynamic limitations.

**Local verification:**

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/reconctx-linux-amd64 ./cmd/reconctx
sha256sum dist/reconctx-linux-amd64 > dist/reconctx-linux-amd64.sha256
```

If a required dependency prevents `CGO_ENABLED=0`, record it and revisit the dependency rather than silently changing the distribution promise.

**STOP G5:** do not run `gh repo create`, add a GitHub remote, push, publish a release or upload artifacts until the operator explicitly approves publication after reviewing the sanitized tree.

**Commit:** `docs: prepare local reconctx release candidate`

---

## 3. Definition of Done for the MVP vertical slice

- [ ] `plan`, `run`, `resume` and `build` work as contracted.
- [ ] Two approvals are digest-bound; no active shortcut exists.
- [ ] GAU/Katana/Arjun exact supported versions are preflighted and conservative adapters pass every fixture.
- [ ] Scope/canonicalization vectors match the Python oracle.
- [ ] Process groups, cancellation, timeout and partial artifacts pass fake and operator loopback tests.
- [ ] JSONL records validate and every material fact resolves to Evidence.
- [ ] Candidate inclusion/exclusion/ranking is deterministic and explainable.
- [ ] Handoff is deterministic, portable, checksummed, secret-scanned and smaller at the compact front door.
- [ ] No finding/severity is generated.
- [ ] Agent has no active control path.
- [ ] Full Go/Python/security/race suites pass.
- [ ] Compatibility matrix and dynamic limitations are current.
- [ ] Local release artifacts verify.
- [ ] Publication remains separately approved.

## 4. Explicitly deferred

- HAR/auth/Burp support;
- BBOT importer;
- SQLite or other derived index;
- dashboard/API/daemon/MCP;
- distributed execution;
- non-Linux process semantics;
- plugin ecosystem;
- automatic findings/severity;
- signed/non-interactive approvals;
- external-target acceptance testing.

## 5. Approval requested

Approval should be unambiguous and scoped, for example:

```text
APROVO A IMPLEMENTAÇÃO LOCAL DO MVP RECONCTX CONFORME O PLANO
.hermes/plans/2026-07-13_002250-reconctx-mvp-implementation.md

NÃO APROVO PUBLICAÇÃO/GITHUB NESTE MOMENTO.
```

Any materially changed architecture, tool scope, intensity, active shortcut, dependency posture or publication action requires a new explicit decision.
