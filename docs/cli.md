# CLI Contract

This document defines the v0.1.0 Linux amd64 command surface. `reconctx` is interactive at active-execution boundaries and has no `--yes`, auto-approve, or agent approval path.

## `reconctx plan`

Render and persist an immutable collection plan after offline validation and non-executing tool-metadata preflight.

```text
reconctx plan \
  --target TARGET \
  --seed URL [--seed URL ...] \
  --scope SCOPE.yaml \
  --wordlist WORDLIST \
  [--profile web-blackbox] \
  --workspace ABSOLUTE_PRIVATE_DIR \
  [--out PLAN_PATH] \
  [--gau-path PATH] [--katana-path PATH] [--arjun-path PATH]
```

`--target`, at least one `--seed`, `--scope`, `--wordlist`, and `--workspace` are required. `--target` is one canonical host name and must match the host of at least one seed. Every seed must be a valid URL allowed for active use by the scope document. The scope document is limited to 1 MiB. The wordlist must resolve to a bounded regular file. Planning reads at most 16 MiB from one validated file descriptor, copies the exact bytes to the private run workspace, and binds that copy's absolute path and SHA-256 into the plan; later changes to the source file cannot affect the approved run. Its non-empty line count becomes the per-candidate Arjun request budget. `--out` defaults to the run's `plan.json` inside the workspace and may not escape the workspace.

`web-blackbox` is the only v0.1.0 profile. Its ceilings are:

| Tool | Activity | Rate/s | Concurrency | Parallelism | Timeout | Additional ceiling |
|---|---|---:|---:|---:|---:|---|
| GAU | passive external archive query | 1 | 1 | 1 | 45 s | pinned providers/arguments |
| Katana | active approved | 2 | 1 | 1 | 10 s | depth and origin scope fixed by plan |
| Arjun | active approved | 1 | 1 | 1 | 15 s | at most 25 candidate targets |

The bounded runner adds fixed ceilings to those profile limits: stdout, stderr, and each native output are limited to 16 MiB; stdout and stderr are each limited to 100,000 newline-delimited records and 1 MiB per line. Adapters reject an admitted raw artifact above 16 MiB, invalid UTF-8, or a line above 1 MiB. Timeout or interruption sends `TERM`, allows a 2-second grace period, then escalates containment; any truncation forces partial coverage. A published handoff is limited to 1,024 filesystem entries, 16 MiB per file, and at most 64 `/` separators in any file path.

Tool path flags override `PATH` lookup. Bare tool names are resolved only through the captured, approved `PATH`; relative paths containing `/` are rejected. Empty and duplicate `PATH` entries are removed, and non-absolute entries are rejected. Preflight accepts only the exact versions in `docs/compatibility-matrix-v0.md`, resolves an absolute executable, records its SHA-256 and filesystem identity, and rejects writable or unsafe binaries and parent paths. It reads bounded metadata and never starts a scanner or version-probe process.

The plan stores the sorted effective `KEY=value` environment snapshot, not only the allowlist names. Approval A displays and binds that exact snapshot, and active execution receives those approved values even if the parent process environment changes later.

Scope YAML is strict. A minimal origin allowlist is:

```yaml
mode: allowlist
external_policy: reject
roots:
  - id: fixture_origin
    kind: origin
    value: http://fixture.test:18080
```

Root kinds are `origin`, `host`, and `url_prefix`. Katana's approved `-cs` expression is derived from the specific root that admitted each seed; a `url_prefix` root keeps its path boundary instead of widening to the origin. For that root kind, discovered suffixes containing percent escapes are conservatively excluded so encoded separators cannot widen active scope. `external_policy` is `reject` or `record_only`; neither permits active execution outside an allowlisted root.

## `reconctx run`

```text
reconctx run [--out HANDOFF_PATH] PLAN
```

Open the persisted plan, revalidate its scope and tool identities, display the exact effective behavior, and enter collection approval. Active subprocesses start only after an operator at a terminal enters the full displayed digest and approves.

The collection prompt supports `approve` and `cancel`. EOF, a non-terminal input, a mismatched digest, or any plan/tool drift exits without launching a child. `--out` is optional and must remain inside the plan workspace.

After bounded GAU/Katana capture, normalization creates a deterministic Arjun queue and a second digest. The Arjun prompt supports:

- `approve`: execute exactly the displayed queue;
- `skip`: record an explicit coverage gap and compile without Arjun;
- `cancel`: persist cancellation and stop scheduling.

v0.1.0 does not edit or reorder the generated queue. Changing a candidate, method, location, wordlist, request budget, limit, or command requires a new reviewed plan/run.

An explicit `skip` completes the command after publishing a partial handoff, and the persisted run state is terminal `partial` rather than `success`.

## `reconctx resume`

```text
reconctx resume --workspace ABSOLUTE_PRIVATE_DIR [--out HANDOFF_PATH] RUN_ID
```

Resume a persisted run only from a safe checkpoint. It can:

- continue `planned` or `awaiting_collection_approval` after a fresh collection decision;
- continue `awaiting_arjun_approval` from the persisted, revalidated queue after a fresh Arjun decision;
- finish or verify an offline `compiling` checkpoint;
- verify and report the handoff for a `success` checkpoint.

Persisted collection, normalization, Arjun-execution, and other in-flight checkpoints are rejected as unsafe to resume. `preflight_failed`, `partial`, `failed`, `interrupted`, and `cancelled` are terminal and are never retried implicitly; a new reviewed plan is required. If the original run used a custom handoff path, pass the same `--out` when resuming.

## `reconctx build`

```text
reconctx build --workspace ABSOLUTE_PRIVATE_DIR --run RUN_ID [--out OUTPUT_PATH]
```

Compile a persisted workflow into a portable handoff. `build` is offline: it reads the already-normalized workflow and admitted raw sources, verifies Evidence references and integrity, stages and syncs the complete package, then publishes the directory with one no-replace rename. It never resolves or launches a scanner. The accepted run states are `compiling`, `success`, `partial`, `failed`, and `interrupted`; every state still requires a complete, plan-matching workflow.

`OUTPUT_PATH` defaults to `handoff/RUN_ID` inside the workspace. It must be a safe output location accepted by the workspace/export policy. Existing finalized artifacts, symlinks, hardlinks, special files, traversal paths, and integrity failures are rejected rather than overwritten or followed.

## Approvals and digests

Approval values use exactly:

```text
sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

The decision line is exactly `<decision> <digest>`, followed by a private operator label. After surrounding whitespace is trimmed, current validation requires 1–128 bytes and rejects Unicode control and format characters. Collection decisions are `approve` or `cancel`; Arjun decisions are `approve`, `skip`, or `cancel`. The prefix and all 64 lowercase hexadecimal characters are required. Collection approval displays and covers the scope path/hash, wordlist path/hash, seeds, profile and policies, resolved tool paths, binary SHA-256/mode/UID/GID/device/inode, versions, argument vectors, per-tool and global limits, output paths, exact effective environment plus allowlist, and workspace root. Arjun approval displays and covers the ordered canonical queue, including the exact private Arjun, wordlist, and native-output paths; exact argv; method/location; wordlist hash; request budget; and limits. The portable candidate-decision projection redacts the executable, wordlist, and native-output paths.

Approval records are append-only. A changed digest is never accepted by confirmation alone; the operator must enter the full current digest for a supported checkpoint or create a new reviewed plan/run.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Command completed successfully, including an explicitly skipped Arjun phase where the handoff records the gap |
| 1 | Operational, preflight, execution, integrity, or compilation failure |
| 2 | Invalid command or usage |
| 3 | Active approval was not granted: paused, declined, or cancelled; no unapproved child is launched |
| 130 | Interrupted; state and partial evidence were persisted where possible |

Tool process exit codes and semantic coverage are preserved separately; a child exit code of zero does not by itself mean complete coverage.

## Version

```text
reconctx --version
```

Prints the version embedded at build time. Release binaries must report `v0.1.0`; development builds may report a development suffix.

## Workspace handling

Use an absolute, operator-owned private directory. `reconctx` creates unique run directories, uses rooted filesystem operations and atomic writes, and does not package local absolute paths or credentials into the portable handoff. Never place private captures in the repository.
