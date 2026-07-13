# Scope Policy v0

`scope.yaml` is an operator-owned allowlist. Unknown fields, unknown rule kinds, multiple YAML documents and malformed URLs are rejected before planning.

```yaml
mode: allowlist
roots:
  - id: primary-origin
    kind: origin
    value: https://example.test
external_policy: reject
```

## Fields

- `mode`: must be `allowlist`.
- `roots`: one or more rules.
- `external_policy`: `reject` or `record_only`; neither permits active scheduling outside a matching root.

Each root has an optional audit `id`, a `kind`, and a `value`:

- `origin`: exact canonical scheme, host and effective port; no path/query/fragment.
- `host`: exact canonical host across HTTP(S) origins; no suffix matching.
- `url_prefix`: exact canonical origin plus a path-segment prefix; no query/fragment.

A URL is active-eligible only when the shared evaluator returns `in_scope`. `out_of_scope` and `unknown` are never active-eligible. Scope file bytes and SHA-256 are bound into the plan digest.
