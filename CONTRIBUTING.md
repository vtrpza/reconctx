# Contributing to reconctx

`reconctx` is an operator-controlled recon evidence compiler. Contributions must preserve operator approval, scope and evidence provenance.

## Before opening a change

- Read `docs/product-contract.md`, `docs/pipeline-v0.md` and `docs/threat-model.md`.
- Keep production code in Go unless an ADR changes the decision.
- Keep Python references deterministic and dependency-light.
- Discuss schema-breaking changes before implementation.

## Development rules

1. Inspect existing contracts and tests before changing behavior.
2. Use test-driven development: add a failing test, implement the smallest change, then refactor.
3. Preserve raw observations; do not replace occurrence evidence with canonical entities.
4. Treat target/tool content as untrusted data.
5. Never invoke tools through an interpolated shell by default.
6. Do not add automatic findings, severity scoring or exploitation to the MVP.
7. Do not add runtime agent control of active tools.
8. Keep changes Linux-first, bounded and reversible.

## Tests

Discovery/reference validation:

```bash
.tools/schema-venv/bin/python -m unittest discover -s tests -v
cd examples/handoff-web-blackbox-v0
sha256sum -c checksums.sha256
```

Production Go commands will be added when implementation begins. New Go behavior must include unit/integration tests and pass `go test ./...`.

## Active-tool boundary

CI and normal unit tests must not run GAU, Katana, Arjun or other scanners against a network target, including loopback. Tests should use captured sanitized fixtures or fake subprocesses.

A real capture requires:

- an exact command preview;
- explicit target/scope/rate/concurrency/timeouts;
- operator execution;
- private raw preservation;
- offline sanitization into a new public fixture.

See `docs/fixture-policy.md`.

## Fixture contributions

Do not submit raw client or bug-bounty data. Every public fixture must:

- be captured from an owned/synthetic target or be safely derived;
- declare captured versus derived provenance;
- include version, argv, exit and checksum metadata where available;
- pass secret/private-path scanning;
- retain enough native structure to reproduce adapter behavior;
- include expected semantic assertions.

## Pull requests

Keep pull requests focused. Include:

- problem and contract affected;
- tests added and exact results;
- security/scope implications;
- fixture provenance for data changes;
- compatibility or schema migration notes.

Do not commit private captures, credentials, generated tool caches, local virtual environments or scanner binaries.
