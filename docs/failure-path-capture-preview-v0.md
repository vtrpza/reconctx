# Failure-Path Matrix and Operator Command Preview v0

**Status:** FP-013/FP-014/FP-015 captured, checksum-verified, sanitized and fixture-tested  
**Execution owner:** operator  
**Active boundary:** `127.0.0.1:18080` only  
**Harness:** `scripts/capture-failure-path.sh`

## 1. Purpose

Close the remaining fixture-corpus gaps without weakening the control boundary. Offline/synthetic cases are generated and tested by Hermes. Real scanner behavior is captured only when the operator runs an explicitly previewed case.

No new GAU external query is required.

## 2. Coverage matrix

| ID | Failure/edge case | Source | Network | Execution owner | Expected contract |
|---|---|---|---:|---|---|
| FP-001 | missing Katana/Arjun | fake PATH/preflight | no | Hermes, during implementation tests | fail before activity; remediation/path shown |
| FP-002 | unsupported tool version | fake version binary | no | Hermes | raw version preserved; adapter blocked as unsupported |
| FP-003 | non-zero exit with partial output | fake subprocess | no | Hermes | partial raw/records preserved; exit and semantic status separate |
| FP-004 | exit 0 with provider/tool error | derived GAU/fake | no | Hermes | diagnostic/coverage gap; never `success_nonzero` by exit alone |
| FP-005 | malformed/truncated JSON/JSONL | derived fixture | no | Hermes | raw preserved; bounded diagnostic; `unsupported_format`/partial |
| FP-006 | zero result and absent requested native file | captured Arjun zero fixture | already captured | none | `success_zero`; stdout Evidence; coverage not universal absence |
| FP-007 | ANSI/control/invalid UTF-8 | fake bytes | no | Hermes | raw bytes preserved; display escaped; no terminal replay |
| FP-008 | oversized line/file/depth | generated data | no | Hermes | parser limits; partial diagnostic; no unbounded allocation |
| FP-009 | path traversal/symlink/FIFO/device | temp filesystem | no | Hermes | block before managed read/write escapes root |
| FP-010 | child/grandchild ignores TERM | `spikes/fake_tool.py` | no | Hermes | TERM→KILL; streams preserved; no process leak |
| FP-011 | disk/write failure | bounded temp filesystem/fake writer | no | Hermes | atomic state; no deliverable corrupt package |
| FP-012 | resume with hash mismatch | mutated copied fixture | no | Hermes | resume blocked; original finalized artifact untouched |
| FP-013 | Katana interrupted mid-crawl | real Katana + loopback target | captured 2026-07-12 | **operator** | exit 124; 3 partial JSONL records; checksums valid; listener/children closed after bounded harness recovery |
| FP-014 | Arjun interrupted mid-run | real Arjun + loopback target | captured 2026-07-13 | **operator** | exit 124; stdout preserved; native JSON absent; classified partial/interrupted, not zero |
| FP-015 | Arjun request timeout | real Arjun + `/slow` loopback | captured 2026-07-13 | **operator** | exit 1 before outer timeout; internal `AttributeError`; classified tool-error/failed, no absence claim |
| FP-016 | out-of-scope URL in target content | existing deterministic root page | future runner test | operator for real tool | record-only or tool-scope suppressed; never Arjun candidate |
| FP-017 | handoff budget exceeded | generated normalized records | no | Hermes | deterministic segmentation; Evidence retained |
| FP-018 | prompt-injection marker | existing target/derived records | no for compiler test | Hermes | label as untrusted data; never instruction |

## 3. Active cases prepared for the operator

The harness defaults to preview and refuses:

- unknown cases;
- non-loopback target configuration;
- an existing listener on port 18080;
- append/overwrite of an existing capture directory;
- missing tool wrappers/wordlist.

In `--execute` mode it:

1. starts `fixture_target.app` bound explicitly to `127.0.0.1`;
2. verifies the socket is not wildcard-bound;
3. runs one bounded case;
4. records command/version/stdout/stderr/native output/exit/timestamps;
5. writes manifest/environment/checksums;
6. stops the target;
7. refuses success if port 18080 remains open.

### Preview commands — no scanner execution

```bash
bash scripts/capture-failure-path.sh katana-interrupt --preview
bash scripts/capture-failure-path.sh arjun-interrupt --preview
bash scripts/capture-failure-path.sh arjun-timeout --preview
```

### Operator execution record

All three operator-owned cases are complete. Private capture directories are immutable and the commands must not be rerun or overwritten:

```text
katana-interrupt  -> KAT-INTERRUPTED-LOOPBACK
arjun-interrupt   -> ARJUN-INTERRUPTED-LOOPBACK
arjun-timeout     -> ARJUN-REQUEST-TIMEOUT-LOOPBACK
```

Expected private outputs:

```text
private-captures/failure-paths/
├── KAT-INTERRUPTED-LOOPBACK/
├── ARJUN-INTERRUPTED-LOOPBACK/
└── ARJUN-REQUEST-TIMEOUT-LOOPBACK/
```

Each case contains:

```text
command.txt
version.txt
stdout.raw
stderr.raw
exit-code.txt
environment.json
manifest.json
socket-before.txt
socket-after.txt
checksums.sha256
native-output.json[l]    # only if the tool created it
```

## 4. Exact bounded behavior

### FP-013 Katana interruption

- target: loopback root only;
- crawl scope: exact `127.0.0.1:18080`;
- depth: 2;
- rate: 1 request/s;
- concurrency: 1;
- parallelism: 1;
- request timeout: 10s;
- outer interruption: `SIGINT` after 3s;
- force-kill fallback: 2s later.

### FP-014 Arjun interruption

- target: `/api/search` loopback;
- method: GET;
- deterministic 9-entry wordlist;
- rate: 1 request/s;
- threads: 1;
- request timeout: 15s;
- outer interruption: `SIGINT` after 6s;
- force-kill fallback: 3s later.

### FP-015 Arjun request timeout

- target: `/slow?delay_ms=5000` loopback;
- deterministic target delay: 5s maximum;
- Arjun request timeout: 1s;
- rate: 1 request/s;
- threads: 1;
- outer safety timeout: 30s;
- force-kill fallback: 3s later.

The manifest records outer timeout (`exit 124`) separately. Adapter review must inspect stdout/stderr to determine request-level timeout semantics.

### FP-013 operator observation

The Katana command completed its bounded interruption with exit `124` and wrote three valid partial JSONL records. Finalization initially stalled because the loopback fixture target was a non-interactive background child with `SIGINT` ignored; cleanup sent `SIGINT` and waited without a deadline. The stopped harness/target process group was identified exactly, terminated without touching unrelated processes, and then completed its metadata/checksum finalization. Port `18080` and process-leak checks passed afterward.

The harness now uses a non-reentrant bounded cleanup (`TERM` → `CONT` → `KILL` fallback) and has offline regression self-tests. It also parses version banners semantically. The pre-fix private Katana manifest has an empty `tool_version`; immutable `version.txt` records `v1.6.1`. Sanitized/derived metadata must document that correction rather than rewrite the private capture.

### FP-014/FP-015 operator observations

FP-014 exited `124` after Arjun reached chunk processing. Stdout was preserved, stderr was empty and the requested native JSON was absent. This demonstrates interrupted/partial execution only; detected parameters and recall are unknown.

FP-015 exited `1` in 4.149 seconds, before the 30-second outer safety timeout. During deterministic `-T 1` testing against the delayed loopback endpoint, Arjun raised `AttributeError: 'str' object has no attribute 'status_code'` in stability initialization. This demonstrates a tool-error path, not a zero-result scan and not parameter absence.

Both pre-fix Arjun manifests misparsed `127.0.0` from help text as the version because `--version` is unsupported. Runtime stdout records `v2.2.7`; public manifests document the correction and preserve the private originals. The harness parser now rejects bare numeric/IP tokens and uses semantic version context/runtime banners.

Public sanitized fixtures:

- `fixtures/cases/katana/v1.6.1/KAT-INTERRUPTED-LOOPBACK/`;
- `fixtures/cases/arjun/2.2.7/ARJUN-INTERRUPTED-LOOPBACK/`;
- `fixtures/cases/arjun/2.2.7/ARJUN-REQUEST-TIMEOUT-LOOPBACK/`.

## 5. Review after operator capture

Offline review completed:

1. [x] checksums and socket-after evidence verified;
2. [x] tool-specific exit/stdout/stderr/native behavior inspected;
3. [x] selective copies written to new sanitized fixture trees;
4. [x] all 31 private files hash-verified unchanged during copy;
5. [x] public origins labeled `copied-from-loopback-capture` with source manifest hashes;
6. [x] expected semantics and fixture regression tests added;
7. [x] credentials, host paths and private-data patterns scanned;
8. [x] tool contract matrix and version-parser gaps updated.

## 6. Stop conditions

Stop and do not continue to the next case if:

- the script reports a wildcard bind;
- the target is not exactly `127.0.0.1:18080`;
- any command differs from preview;
- the capture directory already exists;
- a child/listener remains;
- checksums fail;
- an unexpected external callback/request is observed.

All three active commands were executed by the operator against the bounded loopback fixture only. They must not be rerun and do not authorize any external target or future changed command.
