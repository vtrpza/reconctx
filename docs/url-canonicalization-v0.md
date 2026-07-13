# URL Canonicalization and Identity v0

**Policy ID:** `url-canonicalization/v0`  
**Status:** approved discovery contract  
**Reference implementation:** `reference/canonicalization_v0.py`  
**Machine-readable vectors:** `fixtures/canonicalization/v0/vectors.json`

## 1. Purpose

Canonicalization v0 correlates observations without destroying evidence or asserting equivalence the tools did not prove.

The policy must satisfy these fixture-derived requirements:

- GAU URLs are historical and do not contain a method;
- Katana records current request method, URL and response observation;
- Arjun's `JSON` label is a tool mode, while the transport is HTTP POST with a JSON body;
- repeated query parameters and raw ordering must remain recoverable;
- one endpoint can have multiple raw URL observations;
- Arjun can miss parameters accepted by the deterministic target;
- zero-result runs and partial provider runs remain auditable.

## 2. Three URL representations

Every accepted URL may produce three distinct values:

| Value | Query | Fragment | Purpose |
|---|---|---|---|
| `url_raw` | exact | exact | immutable evidence value |
| `canonical_observation_url` | normalized, original order/repetitions retained | removed | compare an observed URL form |
| `canonical_route_url` | excluded | removed | endpoint identity input |

Example:

```text
url_raw:
  HTTP://Example.COM:80/users?id=1&id=2#profile

canonical_observation_url:
  http://example.com/users?id=1&id=2

canonical_route_url:
  http://example.com/users
```

No raw representation is overwritten by a derived value.

## 3. Accepted inputs

Canonicalization accepts only absolute `http` and `https` URLs.

Reject with a structured parse warning/error:

- relative references;
- unsupported schemes;
- missing authority/host;
- userinfo (`user:pass@host`);
- invalid/out-of-range ports;
- malformed percent escapes;
- backslashes in the URL;
- ASCII control characters;
- invalid IDNA hostnames;
- ambiguous/non-standard numeric IP representations rejected by the strict IP parser.

Rejected input remains available through raw evidence and yields no Endpoint entity until an adapter can safely parse it.

## 4. Scheme, host and port

1. Lowercase scheme.
2. Lowercase DNS host.
3. Strip a terminal DNS root dot.
4. Convert Unicode DNS labels using UTS #46, non-transitional processing, STD3 rules and IDNA ASCII output.
5. Parse IPv4/IPv6 strictly; compress IPv6 and lowercase hexadecimal digits.
6. Bracket IPv6 in rendered origins.
7. Remove default port `80` for HTTP and `443` for HTTPS.
8. Preserve non-default ports.

Examples:

```text
HTTP://Example.COM:80/        → http://example.com/
https://example.com:8443/     → https://example.com:8443/
http://[2001:0db8::1]:80/a    → http://[2001:db8::1]/a
```

## 5. Path

1. Empty path becomes `/`.
2. Normalize Unicode input to NFC, then UTF-8 percent-encode non-ASCII bytes.
3. Uppercase percent-escape hex digits.
4. Decode percent-encoded RFC 3986 unreserved bytes only.
5. Never decode reserved bytes such as `%2F` into `/`.
6. Remove dot segments using RFC 3986 section 5.2.4.
7. Preserve path case.
8. Preserve repeated slashes.
9. Preserve the trailing slash.
10. Do not infer route templates in v0.

Therefore these remain distinct endpoint paths:

```text
/a
/a/
/A
/a//b
/users/1
/users/2
```

Adapters may later attach an externally evidenced `route_template`, but v0 never invents one.

## 6. Query

Query is excluded from Endpoint identity but retained in Observation identity.

Rules:

1. Preserve whether `?` was present, including an empty query.
2. Split pairs only on `&`.
3. Split each component on its first `=`.
4. Preserve pair order.
5. Preserve repeated names.
6. Preserve blank names and blank values in the observation representation.
7. Preserve the distinction between `a` and `a=` through `has_equals`.
8. Do not translate `+` into space; generic URI canonicalization is not form decoding.
9. Apply NFC/percent normalization independently to names and values.
10. Do not sort pairs for identity.

Example parsed representation:

```json
[
  {"index": 0, "name": "id", "value": "1", "has_equals": true},
  {"index": 1, "name": "id", "value": "2", "has_equals": true},
  {"index": 2, "name": "debug", "value": null, "has_equals": false},
  {"index": 3, "name": "empty", "value": "", "has_equals": true}
]
```

`query_multiset_key` or sorted-query views may be produced later only as explicitly heuristic correlation aids. They are not v0 identity inputs.

## 7. Fragment

Fragments do not reach an HTTP server and are removed from both canonical URLs used by the backend endpoint model.

The adapter still records:

- `fragment_present`;
- `fragment_raw`;
- warning `fragment_removed`.

Client-side/DOM routing is outside this backend-oriented v0 and must use a future explicit asset type rather than changing endpoint identity silently.

## 8. Method and body semantics

HTTP method is uppercased when known. Method is part of Endpoint identity.

Unknown method is represented as `null` in records and `*` only in the ID hash material.

### Tool mapping

| Source | Source label | HTTP method | Body kind | Parameter location |
|---|---|---|---|---|
| GAU | absent | unknown | unknown | query only when present in URL |
| Katana | `GET`, etc. | source method | observed if available | derived only from evidence |
| Arjun | `GET` | GET | none | query |
| Arjun | `POST` | POST | form | form |
| Arjun | `JSON` | POST | JSON | JSON |

A GAU URL and Katana GET URL with the same route do not share an Endpoint ID because GAU does not prove the method. They may receive a deterministic `same_route_as` relationship, while historical and current observations remain separate.

## 9. Endpoint identity

Identity material:

```text
reconctx-ep-v0 NUL method-or-* NUL canonical_route_url
```

ID:

```text
ep_sha256_<full lowercase SHA-256 hex>
```

Properties:

- full SHA-256 is retained; no truncation;
- query and fragment are excluded;
- method, scheme, host, effective port and path are included;
- body kind/content type are observations, not endpoint identity in v0;
- path case, repeated slash and trailing slash remain significant.

Examples:

```text
GET  https://example.com/users  ≠ POST https://example.com/users
*    https://example.com/users  ≠ GET  https://example.com/users
GET  https://example.com/users  = GET  https://example.com/users?id=1
```

## 10. Parameter identity

Identity material:

```text
reconctx-param-v0 NUL endpoint_id NUL location NUL NFC(parameter_name)
```

ID:

```text
param_sha256_<full lowercase SHA-256 hex>
```

Locations:

```text
query | form | json | header | cookie | path | unknown
```

Parameter names remain case-sensitive. Therefore JSON `id`, JSON `ID`, form `id` and query `id` are four distinct parameter identities.

A Parameter entity represents a name/location candidate. Acceptance, reflection or semantic effect belongs to observations and must not be inferred from entity existence.

## 11. Other record IDs

| Record | ID strategy |
|---|---|
| Run | occurrence ID: `run_<UUIDv7>` in production; deterministic fixture ID allowed |
| ToolExecution | occurrence ID: `tx_<UUIDv7>` in production; deterministic fixture ID allowed |
| Asset | SHA-256 of asset kind + canonical value |
| Endpoint | deterministic rule in section 9 |
| Parameter | deterministic rule in section 10 |
| Evidence | SHA-256 of execution ID + artifact SHA-256 + canonical locator |
| Observation | SHA-256 of execution ID + type + subject ID + ordered evidence IDs |
| Relationship | SHA-256 of source ID + relationship type + target ID + ordered evidence IDs |

Occurrence IDs prevent separate executions from collapsing. Deterministic entity IDs allow run-to-run correlation.

## 12. Deduplication and correlation

Normalization never deletes evidence.

- identical entity IDs merge entity views;
- each raw occurrence produces or references an Observation;
- duplicate GAU lines remain separate evidence locators;
- byte-identical Katana stdout and output may share one content artifact digest but preserve both artifact roles;
- historical, current and bruteforced observations never merge semantically;
- unknown-method endpoints remain distinct from method-specific endpoints;
- a `same_route_as` relationship may aid correlation without claiming method equality.

## 13. Scope evaluation order

1. Parse raw URL.
2. Canonicalize scheme/host/effective port/path.
3. Evaluate scope against canonical origin/host/path.
4. Record `in_scope`, `out_of_scope` or `unknown` on the Observation.
5. Never schedule `out_of_scope` or `unknown` observations for active tools.

Both the raw URL and scope rule/reference used for the decision remain auditable.

## 14. Fixture-derived decisions

| Fixture observation | Canonicalization/schema consequence |
|---|---|
| GAU text has URL only | method remains unknown; provider set is execution-level provenance |
| GAU duplicates records | entity may merge; line-level Evidence remains separate |
| GAU JSON can silently drop extensionless paths | adapter contract/version warning, not canonicalization |
| Katana emits method + endpoint + response | current HTTP Observation with known Endpoint method |
| Katana stdout equals native output | artifact content may dedupe; roles stay distinct |
| Arjun `JSON` mode uses POST JSON | source label and normalized method/body are both stored |
| Arjun misses `debug` ground truth | detected Parameter is an observation, never complete ground truth |
| Arjun zero creates no output file | `success_zero` ToolExecution with stdout Evidence |

## 15. Versioning

Any change that can alter a canonical URL, entity ID or parameter ID requires a new policy ID (`url-canonicalization/v1`, etc.).

Adapters and manifests must record the exact policy ID. Reprocessing old raw evidence with a newer policy creates a new derived dataset; it does not mutate previous normalized artifacts.

## 16. Verification

Run:

```bash
python3 -m unittest tests.test_canonicalization_v0 -v
```

The reference implementation is executable specification only. Production implementations in another language must pass the same machine-readable vectors before claiming compatibility.
