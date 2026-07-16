# Competitive Baseline v0 — BBOT and reconFTW

**Status:** complete for pre-implementation positioning  
**Method:** official documentation plus pinned source inspection  
**Execution boundary:** neither framework nor any scanner was executed  
**Machine score:** `benchmarks/competitive-baseline-v0.json`  
**Pivot threshold:** at least 80% of the exact acceptance story

## 1. Decision

Do **not** pivot `reconctx` into a BBOT or reconFTW companion.

Proceed with:

> independent compiler-first core + minimal first-party GAU/Katana/Arjun runner + BBOT `output.json` importer after the initial vertical slice.

Static coverage against AC-001…AC-014:

| Candidate | Score | Coverage | Threshold result |
|---|---:|---:|---|
| BBOT stable v3.0.0 | 6.5/14 | 46.4% | below 80%; no pivot |
| reconFTW main | 4.0/14 | 28.6% | below 80%; no pivot |

The score measures product-contract coverage, not scanner quality or recon breadth.

## 2. Sources pinned

### BBOT

```text
Repository: blacklanternsecurity/bbot
Branch/tag: stable / v3.0.0
Commit: 5355bd1f968b73f06f9d99ada2be5c095e6c5dbe
Local source: .tools/competitive/bbot/
```

Official sources reviewed:

- <https://www.blacklanternsecurity.com/bbot/Stable/scanning/events/>
- <https://www.blacklanternsecurity.com/bbot/Stable/scanning/output/>
- <https://www.blacklanternsecurity.com/bbot/Stable/scanning/advanced/>
- `.tools/competitive/bbot/bbot/modules/output/json.py`
- `.tools/competitive/bbot/bbot/scanner/scanner.py`
- `.tools/competitive/bbot/bbot/modules/wayback.py`

### reconFTW

```text
Repository: six2dez/reconftw
Branch: main
Commit: 9a5f12f62cb18d2a948a6a3c820219a8229bcfb5
Local source: .tools/competitive/reconftw/
```

Official/source material reviewed:

- <https://docs.reconftw.com/output/output>
- repository README at the pinned commit;
- `.tools/competitive/reconftw/modules/web.sh:1269-1398`;
- `.tools/competitive/reconftw/modules/modes.sh`;
- `.tools/competitive/reconftw/tests/integration/test_checkpoint.bats`.

## 3. Scoring method

Each acceptance criterion receives:

- `1.0`: directly covers the criterion as written;
- `0.5`: useful partial mechanism, but a reconctx layer is still required;
- `0.0`: absent or conflicts with the product contract.

The threshold is intentionally strict. Execution breadth does not compensate for missing operator control, evidence preservation, semantic separation, or handoff quality.

## 4. Acceptance matrix

| AC | Requirement | BBOT | reconFTW | Rationale |
|---|---|---:|---:|---|
| AC-001 | plan before execution with exact commands/limits | 0.5 | 0.5 | both expose dry-run/config visibility; neither proves the complete per-child effective command contract required by reconctx |
| AC-002 | explicit approval before active phases | 0.5 | 0.0 | BBOT has scan confirmation/`--yes`, but not digest-bound two-phase approval; reconFTW is automation-oriented without the required gate |
| AC-003 | preserve native stdout/stderr/exit/argv/version | 0.5 | 0.5 | both keep useful outputs/logs, but not the immutable per-ToolExecution evidence contract |
| AC-004 | historical versus current observations | 0.5 | 0.0 | BBOT event type/module/tags can support mapping; reconFTW aggregation generally flattens source semantics |
| AC-005 | no destructive deduplication | 0.5 | 0.0 | BBOT UUID/graph is useful but internal dedup/collapse semantics differ; reconFTW uses append/deduped flat lists |
| AC-006 | explicit scope enforcement | 1.0 | 0.5 | BBOT has targets, seeds, blacklist and scope distance; reconFTW has scope filters but less explicit per-observation scope provenance |
| AC-007 | second decision for exact Arjun queue | 0.0 | 0.0 | neither implements the required digest-bound approve/skip/cancel queue gate |
| AC-008 | deterministic bounded candidate phase | 0.5 | 0.5 | both offer limits/config; neither emits our ranked queue, reasons and approval digest |
| AC-009 | partial success with explicit coverage gaps | 0.5 | 0.5 | both continue across failures; neither directly supplies our exit/semantic/coverage separation |
| AC-010 | cancellation plus recoverable partial evidence | 0.5 | 0.5 | both have lifecycle/cleanup mechanisms; reconctx still needs process-group and immutable-artifact guarantees |
| AC-011 | portable agent-ready handoff | 0.0 | 0.0 | reports/JSON trees are not the evidence-resolvable handoff contract |
| AC-012 | factual claims cite resolvable Evidence IDs | 0.5 | 0.0 | BBOT IDs/UUID/parent graph are importable but lack our raw Evidence locator contract; reconFTW lacks equivalent IDs |
| AC-013 | no unsupported findings/severity | 0.0 | 0.0 | both intentionally support findings/vulnerability or risk outputs; reconctx MVP forbids automatic promotion |
| AC-014 | no agent active-control plane | 1.0 | 1.0 | both can be used as operator-run CLIs; optional APIs/AI reports do not force an agent control plane |

## 5. BBOT assessment

### Strong overlap

BBOT is the closest architectural neighbor:

- newline-delimited `output.json`;
- event type, ID, occurrence UUID and timestamp;
- module/source;
- parent and parent UUID;
- discovery context/path;
- scope distance and tags;
- scan lifecycle states;
- presets, dry-run and scope controls;
- JSON, SQLite and other output modules;
- URL, URL_UNVERIFIED, HTTP_RESPONSE and WEB_PARAMETER event types.

The JSON output module watches all events and serializes `event.json()` one line at a time while preserving the graph (`bbot/modules/output/json.py:8-39`). This makes BBOT an excellent import source.

### Hard gaps

- no Approval B for a deterministic Arjun queue;
- no per-subprocess immutable raw/command/version/exit contract equivalent to ToolExecution + Evidence;
- no reconctx three-form URL/canonical endpoint identity contract;
- no portable `CONTEXT.md`/Evidence locator handoff;
- `FINDING` events carry severity/confidence by design;
- event identity/graph semantics cannot simply replace occurrence/evidence separation;
- scan-name reuse can append to prior output, which needs defensive import handling;
- default scan retention is not a substitute for private immutable evidence preservation.

### Intended importer

A later importer should read BBOT `output.json` without embedding or invoking BBOT. Candidate mapping:

| BBOT field/type | reconctx destination |
|---|---|
| scan ID/timestamp | Run/import metadata |
| event UUID | source occurrence locator |
| event ID/data | source identity hint, never trusted as reconctx ID |
| module/module_sequence | Observation source provenance |
| parent UUID/chain | Relationship provenance |
| scope_distance/tags | scope input + source assertion |
| URL_UNVERIFIED | historical/inferred Observation depending source contract |
| URL/HTTP_RESPONSE | observed candidate only when event semantics support it |
| WEB_PARAMETER | Parameter/Observation when method/location are available |
| FINDING | source assertion only or excluded initially; never automatic reconctx finding |

Import remains deterministic and offline. No BBOT plugin, Python runtime coupling, or runtime agent integration is required.

## 6. reconFTW assessment

### Strong overlap

reconFTW already offers:

- broad orchestration and many source tools;
- Katana and Arjun integration;
- output tree, logs and optional structured logging;
- dry-run;
- checkpoint/resume markers;
- parameter caps/timeouts;
- incremental/diff workflows;
- `assets.jsonl` and reports.

It is strong when the product goal is “run a broad automated recon stack.”

### Hard gaps observed in source

The Arjun path in `modules/web.sh:1269-1398`:

- runs only in deep mode;
- limits input with `PARAM_MAX_URLS` but does not stop for a second operator approval;
- invokes Arjun with text output;
- redirects logs and applies `|| true`;
- parses URLs/parameters through text pipelines;
- merges results back into aggregate URL lists;
- does not preserve the reconctx method/location/evidence identity model.

Checkpoint tests use marker files for completed functions. That is useful resume behavior, but it is not an artifact-hash manifest and does not by itself detect valid partial native output.

The framework also intentionally performs vulnerability checks, hotlisting and risk-oriented reporting, which conflicts with the narrow no-finding/no-severity MVP contract.

### Import posture

A future flat-artifact importer is possible, but lower priority than BBOT because source/occurrence/method/location provenance may already have been flattened. It must mark unavailable semantics as unknown rather than reconstructing them from filenames.

## 7. Why active execution is not required to reject the pivot

The operator retained execution of all active tools. Therefore this baseline did not run either framework.

A runtime test could validate installation, actual output quirks and cancellation. It cannot create product features that are structurally absent:

- two digest-bound approval gates;
- reconctx Evidence locators;
- canonical handoff compiler;
- prohibition on automatic findings;
- exact Arjun candidate policy.

Both static scores are far below 80%, and the missing criteria are central differentiators. No active scan is necessary to decide that reconctx should not become only a companion.

## 8. Final positioning

```text
reconctx core
├── canonical schema/evidence model
├── deterministic compiler
├── compact agent handoff
├── minimal supervised GAU/Katana/Arjun runner
└── offline importers
    ├── BBOT output.json       # first external importer
    ├── HAR                    # authenticated observation importer
    └── reconFTW artifacts     # optional/later
```

The competitive gate is closed for positioning. A BBOT runtime fixture may still be useful when implementing its importer, but it is not a prerequisite for the initial runner.
