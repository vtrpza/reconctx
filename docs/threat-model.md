# Threat Model and Safety Contract v0

**Status:** discovery contract  
**Product mode:** operator-run artifact producer  
**Scope:** Linux-first `web-blackbox` vertical  
**Related:** `docs/product-contract.md`, `docs/product-decisions-v0.md`, `docs/pipeline-v0.md`

## 1. Security objective

The CLI must let an authorized operator supervise external recon tools and produce a trustworthy, portable evidence handoff without:

- expanding network activity beyond approved scope/intensity;
- interpreting target/tool content as instructions;
- leaking credentials or private engagement data;
- executing attacker-controlled shell syntax;
- writing outside the workspace;
- losing provenance during failure/cancellation;
- granting an agent runtime control over active operations.

This is a safety/provenance product, not a sandbox for hostile native binaries. The host operator remains responsible for installing trusted tool binaries.

## 2. Assets

| Asset | Required property |
|---|---|
| target authorization/scope | cannot silently broaden |
| approval decision | binds exact behavior-bearing plan/queue digest |
| credentials/auth context | raw secret never enters normalized/handoff data |
| raw evidence | immutable after finalization; attributable to execution |
| normalized records | deterministic, schema-valid, rebuildable |
| provenance graph | every fact resolves to execution/artifact/locator |
| private workspace | no unintended disclosure or path escape |
| handoff package | portable, sanitized, internally consistent |
| operator terminal | target output cannot inject commands/control sequences |
| child process lifecycle | no orphaned or unbounded tool activity |
| public fixture/repository | no private engagement data or executable hostile artifacts |

## 3. Actors and adversaries

### Trusted within stated limits

- authorized operator;
- reconctx core built from reviewed source;
- local OS primitives and filesystem under the operator account;
- explicitly selected tool binary identity at approval time.

### Untrusted

- target-controlled HTTP content, redirects, headers, bodies, links, filenames, and ANSI bytes;
- archive/provider results;
- native tool stdout/stderr/output—even from a trusted binary;
- imported JSONL/HAR/BBOT artifacts;
- scope/profile/plan files received from elsewhere;
- environment variables and tool config files inherited from the shell;
- filesystem entries not created and held safely by reconctx;
- downstream agent interpretation of packaged content;
- downloaded dependencies/releases until verified.

### Out-of-scope adversary for v0

A privileged local attacker who can modify reconctx memory, ptrace the process, or replace the OS/kernel can defeat local checks. Checksums provide integrity detection inside the workflow, not cryptographic authenticity against a user-account compromise.

## 4. Trust boundaries

```text
[operator input/config]
        │ B1 parse/validate
        ▼
[plan + approvals] ── B2 digest binding ──► [runner]
                                              │
                         B3 argv/env/process  │
                                              ▼
                                      [external tools]
                                              │
                         B4 untrusted bytes   │
                                              ▼
                                      [private raw store]
                                              │
                         B5 bounded parser    │
                                              ▼
                                    [normalized records]
                                              │
                         B6 redaction/select │
                                              ▼
                                       [handoff package]
                                              │
                         B7 data/instruction │
                                              ▼
                                      [downstream agent]
```

Network scope is an additional boundary around every tool request/redirect/candidate.

## 5. Security invariants

1. No network-producing child starts without a matching approval digest.
2. No shell parses tool arguments.
3. Every tool path is absolute, resolved, versioned, and rechecked before execution.
4. Scope is evaluated before scheduling; out-of-scope/unknown is record-only.
5. Raw evidence is never overwritten by normalization or sanitization.
6. Target/tool bytes are never replayed verbatim to a terminal or presented as agent instructions.
7. Managed writes remain below a verified workspace/handoff root without symlink traversal.
8. Secrets are removed before indexing/package compilation; authentication uses only opaque `authctx_...` references.
9. Cancellation stops process groups and preserves recoverable evidence.
10. A package is deliverable only after schema, reference, manifest, secret, and checksum gates pass.
11. Recon observations never become findings/severity automatically.
12. No MCP/API/daemon grants the agent active execution.

## 6. Threat register

### T-01 — Shell/argument injection

**Scenario:** target/config value contains shell metacharacters or a user flag redirects execution through shell behavior.

**Impact:** arbitrary local command execution.  
**Initial risk:** Critical.

**Required controls:**

- use direct exec with argv arrays; never `sh -c`;
- validate structured wrapper flags separately from passthrough argv;
- deny flags that alter target/scope, output roots, config loading, proxying, callbacks, update/install behavior, or shell execution unless modeled explicitly;
- shell-escape only for display, never to reconstruct execution;
- property tests with whitespace, quotes, newlines, `$()`, backticks, semicolons, and leading dashes.

**Residual risk:** trusted external tool may itself interpret a value unsafely; pin/compatibility-test supported versions.

### T-02 — Approval bypass or plan drift

**Scenario:** approved plan differs from effective path/version/argv/scope/limits; a resume reuses stale approval.

**Impact:** unapproved network activity.  
**Initial risk:** Critical.

**Required controls:**

- canonical behavior digest for Approval A and queue digest for Approval B;
- re-resolve and re-stat tools/config immediately before launch;
- invalidate approval on every behavior-bearing change;
- no active `--yes` in v0;
- no child creation while state is awaiting approval;
- append-only approval/supersession audit records;
- resume revalidates scope, artifacts, candidate queue, and approvals.

### T-03 — PATH/config/environment hijacking

**Scenario:** malicious binary/config/module is selected through PATH, `PYTHONPATH`, proxy variables, `LD_PRELOAD`, `HOME`, or tool-specific config.

**Impact:** local code execution, traffic redirection, misleading evidence.  
**Initial risk:** Critical.

**Required controls:**

- resolve absolute realpath and record version/hash/metadata;
- warn/refuse binaries or parent directories writable by an unexpected principal;
- execute the approved resolved path, not the command name;
- adapter-specific environment allowlist;
- clear `PYTHONPATH`/`PYTHONHOME` for isolated Python tools as demonstrated by Arjun;
- model proxy/config usage explicitly in plan digest;
- clear dangerous loader variables;
- disable tool auto-update during runs.

Preflight path policy is fail-closed: every resolved path component must be owned by root or the effective user and must not be group/world writable. The only exception is a root-owned sticky directory such as `/tmp`; the executable itself must be a regular executable file and its hash, mode, owner, device and inode enter the plan digest.

**Residual risk:** binary can be replaced by the same local account between check and exec. Linux implementation should minimize TOCTOU and record the executed file identity; stronger fd-based execution may be evaluated in the spike.

### T-04 — Scope drift, redirect escape, and canonicalization mismatch

**Scenario:** encoded/IDNA/IPv6 URL or redirect is classified differently by reconctx and source tool; target redirects to excluded/internal resources.

**Impact:** unauthorized requests.  
**Initial risk:** Critical.

**Required controls:**

- one versioned canonicalization policy;
- scope evaluation on canonical origin/host/path before scheduling;
- tool-native crawl scope as defense in depth;
- re-evaluate redirect/discovered destinations;
- reject userinfo, backslashes, malformed percent escapes, non-standard numeric IPs, unsupported schemes, and unknown scope;
- explicit policy for DNS/IP/private-network boundaries before authenticated/network expansion;
- differential tests between planner, candidate policy, and adapters.

**Residual risk:** DNS rebinding and tool-internal request behavior cannot be fully controlled by URL filtering alone. v0 documents this and avoids claiming a sandbox boundary.

### T-05 — Malicious ANSI/control output

**Scenario:** target/tool emits terminal escape sequences, carriage-return rewriting, OSC links/clipboard operations, or binary bytes.

**Impact:** terminal spoofing, social engineering, hidden diagnostics.  
**Initial risk:** High.

**Required controls:**

- capture raw bytes without terminal replay;
- render escaped/sanitized previews with byte/line limits;
- never use raw output as a terminal format string;
- preserve digest and locator for original bytes;
- test CSI, OSC, NUL, backspace, CR, bidi and invalid UTF-8.

### T-06 — Prompt injection through target/tool content

**Scenario:** page/output tells an agent to ignore instructions, run commands, reveal files, or reinterpret recon as findings.

**Impact:** unsupported claims, data access, unsafe recommendations.  
**Initial risk:** High.

**Required controls:**

- label all raw/target content `untrusted_target_data`;
- keep data in JSON/string/code boundaries, not instruction sections;
- `CONTEXT.md` begins with trust and evidence rules;
- copy minimal snippets; prefer hashes/locators;
- downstream benchmark requires citations and unsupported-claim scoring;
- no runtime execution interface in the handoff;
- no target text can alter candidate policy or commands.

**Residual risk:** downstream agents may still follow hostile content. Handoff reduces exposure but cannot guarantee model behavior.

### T-07 — Secret leakage

**Scenario:** credentials appear in query strings, headers, HAR, stdout, command argv, environment, tool config, bodies, or public fixtures.

**Impact:** account compromise/engagement disclosure.  
**Initial risk:** Critical.

**Required controls:**

- black-box v0 accepts no secret input;
- future auth uses secret references and opaque `auth_context_id` only;
- private raw workspace mode `0700`, files `0600` by default;
- redaction before indexing and packaging;
- typed redaction for Authorization, cookies, tokens, API keys, proxy credentials, sensitive query names and configured patterns;
- command/environment allowlist and redacted display;
- secret scan blocks handoff/public fixture publication;
- private originals never modified during sanitization;
- manifest declares omitted/withheld evidence.

**Residual risk:** generic entropy/regex scans miss context-specific secrets; operator review remains mandatory before publication.

### T-08 — Path traversal, symlink/hardlink and special-file abuse

**Scenario:** tool/output/import path escapes workspace, follows symlink, overwrites files, or targets FIFO/device/socket.

**Impact:** arbitrary file read/write, blocking, data corruption.  
**Initial risk:** Critical.

**Required controls:**

- create workspace root with restrictive mode and retain trusted directory handle;
- reject absolute imported paths and `..` components;
- no-follow/open-beneath semantics where available;
- reject symlink, device, socket and FIFO inputs/outputs;
- create unique execution directories atomically;
- never reuse/append finalized tool outputs;
- atomic temp-write + fsync + rename for manifests/normalized files;
- validate every handoff relative path before copy;
- do not extract archives in v0 without a dedicated safe extractor.

**Residual risk:** hardlink/TOCTOU behavior needs OS-specific tests in the Go spike.

### T-09 — Resource exhaustion

**Scenario:** huge files/lines, deep JSON, decompression bomb, regex DoS, disk fill, excessive URLs/candidates, or child process fan-out.

**Impact:** host denial of service and incomplete evidence.  
**Initial risk:** High.

**Required controls:**

- streaming parsers with byte, line, depth, record, and field-size limits;
- bounded diagnostics/snippets;
- disk quota/preflight free-space threshold;
- max tools, children, candidates, targets and artifact sizes;
- deterministic candidate cap before Approval B;
- regex policy/timeout or safe engine for scope patterns;
- no compressed import in v0 unless bounded;
- fail partial while preserving safely finalized artifacts.

### T-10 — Orphaned or runaway subprocesses

**Scenario:** tool forks descendants, ignores signals, hangs, or survives CLI exit.

**Impact:** continued unapproved traffic/resource use.  
**Initial risk:** Critical.

**Required controls:**

- isolated process group/session;
- context/timeout cancellation;
- graceful TERM then bounded KILL escalation;
- stop scheduling immediately on cancellation;
- child/descendant leak verification;
- bounded concurrency;
- process status persisted even on interruption where possible;
- Linux tests with fake children/grandchildren that ignore TERM.

**Residual risk:** daemonization/namespace escape by a malicious binary is outside the trusted-tool assumption; optional containment can be evaluated later.

### T-11 — Partial/corrupt output interpreted as complete

**Scenario:** exit 0 masks provider errors, JSON truncates, output file is absent, or stdout duplicates native output.

**Impact:** false absence/completeness claims.  
**Initial risk:** High.

**Required controls:**

- separate process exit code, semantic status, and coverage;
- adapter version contracts from real fixtures;
- `success_zero` requires positive normal-completion evidence;
- preserve provider diagnostics;
- unsupported/malformed raw yields gap, not invented records;
- duplicate artifacts may dedupe content hash while retaining roles;
- run/handoff lists explicit gaps.

### T-12 — Provenance or package tampering

**Scenario:** record, raw file, manifest, or locator changes after derivation.

**Impact:** false evidence citation.  
**Initial risk:** High.

**Required controls:**

- SHA-256 for artifacts/package files;
- immutable finalized artifact state;
- deterministic IDs and rebuild;
- manifest file inventory with size/hash;
- package checksum validation before delivery;
- cross-reference and locator validation;
- future signed releases; optional signed run attestations are post-v0.

**Residual risk:** an attacker controlling the same account can rewrite content and hashes. v0 integrity is corruption/provenance detection, not non-repudiation.

### T-13 — Unsafe fixture/repository publication

**Scenario:** private captures, tokens, engagement names, raw bodies, absolute paths, or licensed third-party binaries enter Git/public releases.

**Impact:** disclosure/legal risk.  
**Initial risk:** Critical.

**Required controls:**

- private-captures excluded from repository by construction;
- sanitized fixture tree is separately generated and scanned;
- allowlist publication manifest;
- no third-party tool binaries redistributed;
- secret/PII/absolute-path scan;
- checksum and structural tests;
- human review before first/public releases;
- Apache-2.0 project license plus dependency/tool license inventory.

### T-14 — Supply-chain compromise

**Scenario:** malicious dependency, tool release, installer, or generated artifact enters build/use.

**Impact:** local compromise and falsified evidence.  
**Initial risk:** High.

**Required controls:**

- do not auto-install/update tools during a run;
- record tool source/version/path/hash where available;
- pinned build dependencies and lockfiles;
- release checksums/signatures and SBOM before public beta;
- minimal dependencies in core;
- adapters treat tools as external, not bundled;
- CI dependency review and reproducible-build investigation.

### T-15 — Agent/recon finding inflation

**Scenario:** endpoint/parameter observation is rendered as vulnerability or severity.

**Impact:** false reports and wasted testing.  
**Initial risk:** High.

**Required controls:**

- schema has no automatic finding/severity transition;
- `bruteforced` means discovery method only;
- summaries use factual language and explicit gaps;
- benchmark counts unsupported claims;
- findings require a separate operator/validation workflow.

## 7. Adapter flag policy

Each adapter defines:

- wrapper-owned flags required for structured output, bounds, scope and paths;
- operator passthrough flags;
- denied/conflicting flags;
- effective argv renderer;
- environment allowlist;
- version compatibility predicate;
- native output contract and zero/failure semantics.

At minimum, reject unmodeled flags that:

- add/change targets or seeds;
- disable scope controls;
- increase rate/concurrency/threads;
- change output outside execution directory;
- load arbitrary config/plugin/template/code;
- set proxy/callback/listener;
- trigger install/update;
- enable headless/browser features not approved;
- execute commands.

Unknown flags fail closed in the initial supported profiles. The operator can still run a source tool directly outside reconctx, but reconctx does not falsely claim supervision/provenance for behavior it cannot model.

## 8. Filesystem safety contract

- workspace and handoff roots are explicit absolute directories;
- managed paths are relative components validated before use;
- parent traversal and symlink following are forbidden;
- execution output directories are unique;
- raw finalization is one-way;
- normalized/handoff output is rebuildable and written atomically;
- selective raw copy reads only verified regular artifacts;
- private workspace permissions are restrictive;
- handoff permissions are explicit and do not imply public safety;
- checksums are generated after final content.

## 9. Secret/redaction pipeline

```text
private raw (restricted)
  → bounded parser
  → typed sensitive-field detection
  → normalized value omission/redaction
  → selective evidence copy/redaction
  → secret/PII/path scan
  → manifest records copied/referenced/withheld status
  → operator publication/delivery review
```

Redaction creates a new artifact with its own hash and a derivation relationship. It never edits private raw in place.

## 10. Security test plan

### Unit/property tests

- argv metacharacters remain one argument;
- denied flags cannot override target/scope/output/intensity;
- plan/queue digest changes on every behavior-bearing change;
- canonicalization/scope differential vectors;
- secret-field and opaque auth-context validation;
- ANSI/control/bidi escaping;
- bounded JSON depth/line/field/record sizes;
- safe relative path and symlink rejection;
- deterministic IDs and package rebuild.

### Fake-process integration tests

- missing binary and unsupported version;
- stdout/stderr interleaving;
- exit 0 with semantic error;
- non-zero with partial native output;
- no native output + explicit success zero;
- malformed/truncated output;
- timeout;
- child/grandchild ignores TERM;
- Ctrl-C during write/parse/compile;
- disk quota/write failure;
- resume after each failure point;
- no leaked children.

### Package tests

- schema validation;
- reference resolution;
- evidence locator resolution;
- manifest size/hash inventory;
- checksum verification;
- secret/PII/absolute-private-path scan;
- no symlink/special file;
- prompt-injection fixture remains quoted/labeled data;
- no finding/severity fields produced.

### Operator-executed real-tool tests

Require separate command previews for:

- Katana interruption/truncation on loopback;
- Arjun timeout/interruption on loopback;
- any provider/external query;
- BBOT/reconFTW active baseline.

## 11. Release-blocking conditions

Do not begin production implementation until threat requirements are mapped into the implementation plan. Do not publish a release if any of these remain:

- shell execution path;
- approval digest bypass;
- scope escape in supported URL vectors;
- managed path traversal/symlink write;
- leaked child after cancellation;
- raw secret in normalized/handoff fixture;
- package hash/reference failure;
- unbounded parser for supported native input;
- target content rendered as executable/instructional content;
- private fixture/publication boundary failure.

## 12. Residual-risk acceptance

The operator explicitly retains responsibility for:

- legal authorization and scope correctness;
- choosing trustworthy external tool binaries;
- executing all active tools during discovery fixtures;
- reviewing commands and approval digests;
- reviewing handoffs before sharing;
- final repository publication.

The product remains responsible for faithfully enforcing and recording approved behavior once production implementation is authorized.

## 13. Security posture summary

The highest-risk boundaries are not the JSON schema. They are:

1. approval-to-effective-command binding;
2. scope consistency across wrapper and external tool;
3. process/environment/filesystem isolation;
4. secret transition from private raw to portable handoff;
5. data/instruction separation for the downstream agent.

All five are mandatory architecture constraints for the production plan. None may be deferred as adapter-specific cleanup.
