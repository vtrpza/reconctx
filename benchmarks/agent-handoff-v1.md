# Agent Handoff Benchmark v1

**Verdict:** `PASS`  
**Protocol:** `benchmarks/agent-handoff-v1-protocol.md`  
**Conditions:** three isolated leaf agents in one parallel batch  
**Model identifier:** unavailable (`?` reported by runtime)  
**Active execution/network:** none

## 1. Result

The redesigned compact `CONTEXT.md` satisfies the handoff acceptance benchmark:

- all ten common questions were answered from `CONTEXT.md` alone;
- no raw, normalized, manifest or agent-view drilldown occurred;
- all 15 full Evidence IDs cited by COMPACT exist in the canonical evidence index;
- no historical/current confusion was found;
- no candidate exclusion was invented;
- no vulnerability was promoted from recon data;
- no material unsupported claim was found in independent review;
- COMPACT used fewer source bytes, API calls and runtime than RAW.

Compact protocol score: **7/7 pass rules**.

## 2. Metrics

| Condition | Allowed files | Unique source bytes | Aggregate bytes reported read | API calls | Runtime | Answer bytes |
|---|---:|---:|---:|---:|---:|---:|
| RAW | 16 | 9,332 | logical unique-byte proxy only | 8 | 309.63 s | 15,910 |
| NORMALIZED | 1 | 69,529 | 139,058 | 3 | 225.76 s | 15,988 |
| COMPACT | 1 | 7,064 | 7,064 | 2 | 148.43 s | 12,753 |

COMPACT deltas:

- **24.3% fewer unique input bytes than RAW**;
- **89.8% fewer unique input bytes than NORMALIZED**;
- **75.0% fewer API calls than RAW**;
- **52.1% lower runtime than RAW**;
- **34.3% lower runtime than NORMALIZED**.

Runtime includes agent/tool overhead and is directional, not a controlled CPU benchmark. Unique source bytes and API calls are the primary efficiency measures.

## 3. Factual assessment

| Capability | RAW | NORMALIZED | COMPACT |
|---|---|---|---|
| Three origins and temporal distinction | complete, but formal scope snapshot unavailable | complete | complete |
| Six Katana endpoints | complete | complete | complete |
| Two unique historical URLs / three occurrences | complete | complete | complete |
| Multi-source origin and `/api/search` | complete | complete | complete |
| Five Arjun candidates | names complete; method/location partially unavailable in native data | complete | complete |
| Tool status and declared gaps | source-limited | complete | complete |
| Candidate exclusions | correctly unknown | correctly unknown | correctly unknown |
| Stable Evidence IDs | unavailable | 15 valid | 15 valid |
| Next-run gaps | conservative | conservative | conservative |
| Confirmed vulnerability | correctly none | correctly none | correctly none |

Independent review found **zero material contradictions** and **zero material unsupported claims** in all three answers.

## 4. Citation validation

Programmatic validation against `normalized/evidence-index.jsonl`:

```text
RAW_FULL_EVIDENCE_IDS=0 VALID=0 UNKNOWN=0
NORMALIZED_FULL_EVIDENCE_IDS=15 VALID=15 UNKNOWN=0
COMPACT_FULL_EVIDENCE_IDS=15 VALID=15 UNKNOWN=0
```

COMPACT also preserved the raw path plus line range or JSON Pointer for each Evidence ID, without opening those files.

## 5. Pass-rule review

| Rule | Result |
|---|---|
| No historical/current confusion | PASS |
| No nonexistent Evidence ID | PASS |
| No finding claim | PASS |
| Common questions answered from `CONTEXT.md` only | PASS |
| COMPACT source bytes lower than RAW | PASS |
| COMPACT API calls no greater than RAW | PASS |
| Missing candidate queue reported as a gap | PASS |

## 6. Important limitation retained

Several run/tool diagnostics do not have their own Evidence ID:

- Arjun fixture false negatives for `debug`;
- GAU provider-attribution limitation;
- pending failure-path coverage;
- absence of a candidate queue;
- absence of confirmed vulnerabilities.

Both NORMALIZED and COMPACT disclosed this rather than inventing citations. This does not invalidate the benchmark, but production design should allow diagnostics to reference source metadata/evidence when the claim is evidence-bearing. Negative corpus statements must remain explicitly bounded.

## 7. Condition-specific observations

### RAW

RAW was accurate but required eight calls across 16 files. It could not prove formal scope, stable execution status, parameter location for all Arjun modes or Evidence IDs. Those were correctly reported as unknown.

### NORMALIZED

NORMALIZED recovered all semantics and citations but required reading a 69,529-byte stream twice, for a reported aggregate of 139,058 bytes.

### COMPACT

COMPACT recovered the same decision-relevant facts from one 7,064-byte file and two API calls. It preserved conservative language and did not drill down despite carrying resolvable locators.

## 8. Preserved outputs

- `benchmarks/results/agent-handoff-v1/raw.md`
- `benchmarks/results/agent-handoff-v1/normalized.md`
- `benchmarks/results/agent-handoff-v1/compact.md`

Their SHA-256 values are recorded in `benchmarks/agent-handoff-v1.json`.

## 9. Gate decision

The Definition-of-Ready item **“benchmark showing value to the agent” is satisfied**.

The benchmark proves the compact handoff provides lower-context, lower-call access to the same material facts while retaining Evidence resolution. Remaining blockers are unrelated to handoff value:

1. operator-captured interruption/timeout fixtures;
2. offline review/sanitization of those captures;
3. final implementation plan and explicit operator approval.
