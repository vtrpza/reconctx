# Agent Handoff Benchmark v0

**Status:** executed  
**Verdict:** `PARTIAL` — provenance/auditability validated; context-efficiency claim not validated  
**Execution:** three isolated leaf agents in one parallel batch  
**Source results:** ephemeral isolated-worker summaries; durable metrics are retained below  
**Machine metrics:** `benchmarks/agent-handoff-v0.json`

## 1. Question

Does the fixture-derived compiled handoff let an agent recover the same facts as raw artifacts while improving semantic clarity, evidence citation, and context efficiency?

## 2. Controlled conditions

Each leaf received the same six questions:

1. enumerate hosts/endpoints;
2. separate historical from currently observed URLs;
3. identify Arjun parameters with method/location;
4. identify multi-source entities;
5. state gaps and prohibited conclusions;
6. cite evidence for factual claims.

The conditions were isolated:

| Condition | Allowed source |
|---|---|
| RAW | only the 16 files under handoff `raw/` |
| NORMALIZED | only `normalized/records.jsonl` |
| HANDOFF | start at `CONTEXT.md`; inspect only files referenced inside the package |

All target/tool content was explicitly treated as untrusted data. Agents could not consult project docs, schemas, tests, source, builder code, or external files.

The delegation runtime did not expose the concrete child model identifier (`Model: ?`). All three children were launched in the same batch under the same inherited parent configuration, but this benchmark must not claim cross-provider reproducibility.

## 3. Objective input/output metrics

| Metric | RAW | NORMALIZED | HANDOFF |
|---|---:|---:|---:|
| Allowed/package files | 16 | 1 | 29 |
| Allowed/package bytes | 9,332 | 69,529 | 158,757 |
| Front-door bytes | n/a | 69,529 | 662 (`CONTEXT.md`) |
| Agent tool/API calls | 4 | 3 | 13 |
| Delegation runtime | 169.57 s | 180.62 s | 347.24 s |
| Answer bytes | 7,957 | 11,393 | 9,896 |
| Full Evidence IDs cited | 0 | 15 | 15 |
| Unknown full Evidence IDs | n/a | 0 | 0 |
| Raw path/locator citations | 11 paths | unavailable by condition | 15 resolved locators |
| Material unsupported claims after review | 0 | 0 | 0 |

Package size is not the same as actual model context consumed. The current delegation telemetry did not report bytes read. Agent calls and allowed/package bytes are therefore proxies, not token counts.

## 4. Ground truth

The fixture-derived handoff contains:

- 3 origins/hosts;
- 11 canonical endpoints;
- 6 Katana HTTP observations with status 200;
- 2 unique historical GAU URLs represented by 3 occurrences;
- 5 positive Arjun parameters;
- 1 Arjun zero-result target;
- 2 multi-source entities: the loopback origin and `GET /api/search`;
- 15 Evidence records.

No condition was allowed to infer vulnerabilities, current reachability of historical URLs, authenticated behavior, or universal parameter absence.

## 5. Independent review

### RAW

**Passed:**

- enumerated all relevant origins and endpoint routes;
- correctly separated six Katana responses, historical GAU URLs, and Arjun-only probes;
- found all five Arjun parameters;
- correctly rejected duplicate GAU lines as independent sources;
- identified the two multi-source entities;
- cited 11 existing raw artifacts/locators;
- made no material unsupported claim.

**Source-imposed limitations:**

- no stable Evidence IDs exist in raw;
- the raw Arjun JSON mode records `method: JSON`, so the agent correctly refused to invent the normalized HTTP POST mapping;
- the selected zero-result raw did not provide enough self-contained method/location context, so the agent left them unknown;
- semantic classification depended partly on directory/run labels and manual cross-file reconstruction.

RAW was the smallest input in this tiny fixture and completed with fewer calls than HANDOFF.

### NORMALIZED

**Passed:**

- recovered all 3 origins, 11 endpoints and semantic states;
- correctly separated five Arjun parameters from two Katana query-name parameters;
- identified both multi-source entities;
- cited all 15 valid Evidence IDs with no unknown ID;
- explicitly identified metadata gaps whose records have `evidence_ids: []`;
- made no material unsupported claim.

**Limitations:**

- the agent had to traverse one dense 84-record graph stream;
- it could cite Evidence IDs but was forbidden from resolving them to raw locators;
- input was 7.45x larger than the raw fixture despite better semantics;
- there was no compact navigation/front-door layer.

### HANDOFF

**Passed:**

- recovered all ground-truth facts and limitations;
- cited all 15 valid Evidence IDs;
- resolved every cited ID to a relative raw path/locator;
- distinguished multi-evidence from multi-source;
- verified cited artifact hashes against the manifest;
- made no material unsupported claim.

**Failed efficiency expectation in this run:**

- required 13 calls versus RAW's 4 and NORMALIZED's 3;
- runtime was 347.24 s versus 169.57/180.62 s;
- full package was larger than both alternatives;
- the 662-byte `CONTEXT.md` was too sparse to answer the benchmark without extensive drilldown.

The extra work bought the strongest auditability, but not lower interaction cost.

## 6. Acceptance result

| Requirement | RAW | NORMALIZED | HANDOFF |
|---|---|---|---|
| factual accuracy | PASS | PASS | PASS |
| historical/current distinction | PASS | PASS | PASS |
| parameter method/location | PASS with raw limitations | PASS | PASS |
| correct multi-source reasoning | PASS | PASS | PASS |
| explicit coverage gaps | PASS | PASS | PASS |
| stable Evidence IDs | FAIL/not available | PASS | PASS |
| Evidence ID → raw locator/hash | FAIL/not available | not accessible | PASS |
| fewer-context/interaction proxy than RAW | baseline | FAIL | FAIL |

## 7. Verdict

`PARTIAL`.

The product moat—semantic separation plus evidence-resolvable claims—was demonstrated. The stronger claim that the current compiled handoff is more context-efficient than raw output was not demonstrated on this corpus.

A positive benchmark gate requires both:

1. no regression in factuality/provenance;
2. lower measured bytes read or lower bounded context on a representative corpus, not merely a small `CONTEXT.md` file.

Production runner implementation remains blocked.

## 8. Required redesign before rerun

### Compact front door

`CONTEXT.md` should deterministically include, within its token budget:

- complete origin/endpoint summary for prioritized in-scope records;
- historical/current counts and explicit lists for this scale;
- parameter table with endpoint/method/location/discovery kind;
- multi-source entity table;
- all material gaps;
- compact `Evidence ID → relative locator` rows for facts already surfaced;
- explicit pointers for optional deep JSONL/raw drilldown.

For this fixture, an agent should answer the six questions from `CONTEXT.md` plus at most one compact evidence index read.

### Avoid mandatory projection traversal

The package may retain full records and split projections for portability, but the agent instructions must label them optional drilldown. Do not require reading both `records.jsonl` and every projection.

### Add a deterministic agent view

Evaluate an offline-generated `normalized/agent-view.jsonl` or equivalent denormalized surface view containing one bounded row per prioritized endpoint, with:

- semantic observation states;
- parameter summaries;
- source execution IDs;
- Evidence IDs;
- no raw body or target instructions.

It must remain a derivable view, never a new source of truth.

### Instrument the next run

Each agent must report the exact file list read. The scorer will sum actual bytes and lines. Record:

- model/provider identifier;
- input bytes actually read;
- output bytes;
- tool calls;
- wall time;
- citation validity;
- unsupported claims.

### Use a representative corpus

The raw condition here is only 9,332 bytes. Fixed schema/handoff overhead will naturally be larger. The efficiency claim must be retested on a larger fixture corpus where raw terminal noise and repeated outputs are representative.

## 9. Stop gate

Do not mark the agent benchmark green and do not start the production runner until the compact handoff is regenerated and the benchmark rerun. The schema/evidence model does not need to be discarded; the redesign is in compiled views and navigation.
