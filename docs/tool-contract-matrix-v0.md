# Tool Contract Matrix v0

**Status:** core GAU/Katana/Arjun fixtures plus Katana interruption and Arjun interruption/request-timeout paths validated
**Rule:** contracts derive from real outputs, not documentation alone.

## Summary

| Tool | Version | Native format selected | Fixture status | Adapter status |
|---|---:|---|---|---|
| GAU | 2.2.4 | line-oriented text | canonical + regressions captured | implemented and fixture-tested |
| Katana | v1.6.1 | JSONL | normal scoped crawl + interrupted partial crawl captured | implemented and fixture-tested |
| Arjun | 2.2.7 | JSON + stdout for zero/failure | GET/POST/JSON/zero + interruption + request-timeout failure captured | implemented and fixture-tested |

## GAU 2.2.4

### Semantic role

GAU produces historical/provider-derived URL observations. A URL from GAU is not evidence that an endpoint is currently reachable.

### Canonical invocation

```text
gau --config .reconctx-gau-config-absent [domain] --subs --verbose --providers [provider-set] --threads 1 --timeout 45 --o [new-output-path]
```

For release 2.2.4, canonical adapter input is native text without `--json`.

### Native text contract

- zero or more UTF-8 lines;
- one absolute URL per non-empty line;
- duplicates are possible and must remain in raw evidence;
- no provider field exists per line;
- provider provenance exists only at execution/provider-set level;
- ordering is provider/execution dependent and must not be treated as stable;
- URL presence means historical/provider observation, not live reachability.

Observed canonical fixture:

- raw selected records: 3;
- exact unique URLs: 2;
- duplicate records: 1;
- URL schemes: HTTPS;
- paths: extensionless;
- sensitive query-key hits: 0.

Fixture: `fixtures/cases/gau/2.2.4/GAU-APEX-SUBS-TEXT/`.

### Confirmed release bugs and pitfalls

#### G-2.2.4-JSON-EXTENSIONLESS-DROP

`ProviderConfig()` inserts the empty extension into the blacklist. In release 2.2.4, `WriteURLsJSON()` checks the blacklist without first requiring a non-empty extension. Consequently, every URL with an extensionless path is discarded in `--json` mode.

Evidence:

- providers returned three records;
- all three paths had empty extensions;
- native text retained all three;
- `--json` wrote zero bytes;
- exit code remained zero;
- upstream source after v2.2.4 adds the missing `ext != ""` guard.

Fixture: `fixtures/cases/gau/2.2.4/GAU-JSON-EXTENSIONLESS-DROP/`.

Adapter rule: reject `--json` for GAU 2.2.4. If imported externally, label empty JSON output as potentially incomplete rather than a trustworthy zero result.

#### G-2.2.4-OUTPUT-APPEND

GAU opens `--o` using `O_APPEND|O_CREATE|O_WRONLY`. Reusing a path appends a new execution to old data.

Evidence:

- two identical executions produced sequence `A B B | A B B`;
- first run frequency multiset: `[2,1]`;
- appended raw frequency multiset: `[4,2]`.

Adapter rule:

- output path must not exist before execution;
- refuse overwrite/append unless an explicit resume mode is designed;
- preserve accidental appended output as raw, but mark execution boundary uncertainty.

#### G-2.2.4-EXIT-ZERO-ON-PROVIDER-ERROR

Provider `Fetch()` errors are logged by workers but are not returned to `main()`. Default log level suppresses warnings after config initialization. The process can therefore exit zero with incomplete provider coverage.

Adapter rule:

- always use `--verbose`;
- parse stderr for provider start/error events;
- exit code zero means process completion, not provider completeness;
- run unstable providers in separate executions;
- classify run as `partial` when any selected provider errors or lacks a terminally understood state.

#### G-2.2.4-ISOLATED-MISSING-CONFIG-WARNING

The approved command names `.reconctx-gau-config-absent` inside the new private execution directory. The runner creates that directory immediately before launch and never materializes that path, so GAU cannot consult mutable `~/.gau.toml` state and falls back to its built-in defaults. The resulting missing-config warning is expected and is not a failed execution.

Adapter rule: bind the explicit isolation path in Approval A and classify its exact missing-config warning as `config_defaulted`, not `error`.

### Provider behavior observed

| Provider | Exact content subdomain | Apex/wildcard | Notes |
|---|---:|---:|---|
| Wayback | 0 | 0 | Valid empty response observed |
| OTX | 0 | 1 | Contributed extensionless URL |
| URLScan | 0 | 2 records | Contributed extensionless URLs; duplicates possible across providers |
| Common Crawl | query error | unstable | HTTP 502 observed during query; isolate execution |

### Normalization requirements

For every GAU line:

1. preserve exact raw line and line number;
2. parse only absolute HTTP/HTTPS URLs;
3. emit historical `Observation` linked to canonical `Endpoint`;
4. attach run-level provider set;
5. retain duplicate observations or duplicate evidence references;
6. never infer current reachability;
7. record `source_time` only when the provider/native format actually supplies one — GAU text does not.

## Katana v1.6.1

### Semantic role

Katana produces current crawl observations. A record proves that Katana requested an endpoint during this execution and observed the captured status, not that the endpoint existed before or remains reachable after the run.

### Canonical invocation

```text
katana -u [loopback-or-approved-target] -cs [scope-regex] -d 2 -j -nc -silent -rl 2 -c 1 -p 1 -timeout 10 -or -ob -o [new-output.jsonl]
```

### Native JSONL contract observed

- one valid JSON object per non-empty line;
- top-level fields: `timestamp`, `request`, `response`;
- `request`: `endpoint`, `method`;
- `response`: `status_code`, `headers`, `content_length`;
- `-or -ob` omitted raw request, raw response and body;
- unknown future fields must be preserved/tolerated;
- response headers and timestamps are execution-specific and cannot participate in endpoint identity.

Observed fixture:

- six valid JSONL records;
- six unique endpoint values;
- all methods `GET`;
- all status codes `200`;
- external link present in target HTML was excluded by crawl-scope regex;
- `stdout.raw` and `native-output.jsonl` were byte-identical.

Fixture: `fixtures/cases/katana/v1.6.1/KAT-NORMAL-MINIMAL/`.

### Interrupted JSONL contract

The bounded interruption fixture exited `124` after producing three individually valid JSONL records. `stdout.raw` and the native file were byte-identical. This is a `partial` parse with interrupted execution semantics: retain the valid prefix, preserve exit/timing evidence and prohibit complete-crawl or endpoint-absence claims.

Fixture: `fixtures/cases/katana/v1.6.1/KAT-INTERRUPTED-LOOPBACK/`.

### Adapter rules

1. preserve exact raw line and line number;
2. parse records independently so a malformed line does not discard valid siblings;
3. derive endpoint identity from request method + canonical URL, never timestamp/headers;
4. preserve response status as a run-scoped observation;
5. recognize stdout/native byte equivalence and avoid duplicating evidence;
6. record the scope regex and execution budgets as provenance;
7. do not infer that an out-of-scope URL was absent from target content merely because it was absent from output;
8. on interruption, parse each complete JSONL line, mark coverage partial and never convert an exit `124` into a zero-result claim.

### Pending expansion cases

- empty page;
- redirect chain;
- malformed line amid valid lines;
- timeout and rate-limit behavior.

## Arjun 2.2.7

### Semantic role

Arjun produces parameter-discovery observations. Detected parameters are candidates supported by the tool's heuristics; the absence of a parameter is not proof that the endpoint rejects it.

### Native JSON contract observed

When parameters are found, `-oJ` writes one JSON object:

- top-level keys are target URLs;
- each target value contains `headers`, `method`, `params`;
- `params` is the detected parameter-name list;
- `headers` are generated request defaults, not target response headers;
- native output has no timestamp, confidence, detection basis or request count;
- native output does not retain the supplied wordlist or rate/concurrency budgets.

`method: "JSON"` is an Arjun mode label. The actual transport is HTTP POST with a JSON body. Preserve both source label and normalized transport/body semantics.

The JSON case contained both `Content-Type` and `Content-type`; header normalization must therefore be case-insensitive while raw casing remains preserved.

### Zero-result contract

When no parameter is found:

- process exit code is `0`;
- the requested `-oJ` file is not created;
- stdout states `No parameters were discovered.`;
- stderr is empty.

Absence of the native file is expected for this state and must not automatically become `tool_failed`.

### Interruption and request-timeout contracts

Two absence-of-native-output cases have different semantics:

| Case | Exit | Outer timeout | Native JSON | Required classification |
|---|---:|---:|---|---|
| interrupted during chunk processing | `124` | yes | absent | `partial` / `interrupted`; parameter result unknown |
| deterministic request-timeout path | `1` | no | absent | `tool_error` / `failed`; no parameter result |

In the request-timeout case, Arjun 2.2.7 raised `AttributeError: 'str' object has no attribute 'status_code'` during stability initialization. The process failed before the 30-second outer safety timeout. This is not a zero-result case and supports no parameter-absence claim.

The installed Arjun CLI does not expose a functional `--version`; that invocation prints help containing `127.0.0.1:8080`. Preflight therefore does not execute `--version`. It reads bounded `Name: arjun` and `Version: 2.2.7` distribution metadata from the same environment prefix as an absolute, path-validated entrypoint/interpreter. An unbound `env python` shebang, ambiguous distribution metadata, a bare numeric token, and an IP-like token are rejected. A standalone anchored `v2.2.7` remains a valid parser input for imported runtime evidence, not a preflight probe.

Fixtures: `ARJUN-INTERRUPTED-LOOPBACK/` and `ARJUN-REQUEST-TIMEOUT-LOOPBACK/` under `fixtures/cases/arjun/2.2.7/`.

### Ground-truth differential observed

| Case | Target accepts | Arjun detects | False negatives |
|---|---|---|---|
| GET | `debug`, `q` | `q` | `debug` |
| POST form | `id`, `name` | `id`, `name` | none |
| JSON | `debug`, `filter`, `id` | `filter`, `id` | `debug` |
| Zero | none | none | none |

Fixtures: `fixtures/cases/arjun/2.2.7/`.

### Adapter rules

1. prefer native JSON when it exists;
2. capture stdout because it is the only explicit zero-result artifact;
3. strip ANSI/progress control sequences only in a derived view, never in raw evidence;
4. classify absent `-oJ` + exit `0` + explicit zero message as `success_zero`;
5. classify detected names as observations, not endpoint ground truth;
6. preserve ground-truth differentials in test fixtures to prevent adapters from inventing missing detections;
7. store target URL, mode, wordlist checksum, rate, threads and timeout from execution provenance because native JSON omits them;
8. classify absent native JSON by the full execution tuple—exit, timeout/interruption state and stdout/stderr—not by file absence alone;
9. map exit `124` with interrupted progress to partial/interrupted, preserving unknown parameter result;
10. map exit `1` plus the observed timeout-path traceback to failed/tool-error and prohibit absence claims;
11. resolve version from explicit semantic version context or a `vX.Y.Z` runtime banner, never a bare numeric/IP token.

### Pending expansion cases

- deterministic rate limiting;
- malformed/truncated JSON;
- multiple target URLs in one output.
