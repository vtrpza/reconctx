# Contributing to reconctx

`reconctx` is an operator-controlled reconnaissance evidence compiler. Contributions must preserve approval, scope, process containment, and evidence provenance.

## Before changing behavior

- Read `docs/product-contract.md`, `docs/pipeline-v0.md`, and `docs/threat-model.md`.
- Keep production code in Go unless an ADR changes that decision.
- Keep Python references deterministic and dependency-light.
- Discuss schema-breaking changes before implementation.
- Inspect existing contracts and tests before adding a new abstraction or dependency.

## Development setup

Go 1.24+ is required. Python is used only for executable specifications, fixtures, and repository validation.

```bash
python3 -m venv .tools/schema-venv
.tools/schema-venv/bin/python -m pip install --require-hashes --requirement requirements-dev.lock
go build -o build/reconctx ./cmd/reconctx
```

The development lock pins exact Python versions and verified wheel SHA-256 hashes. Update versions and hashes together, then prove a clean `--require-hashes` installation before review.

## Required checks

```bash
go fmt ./...
go test ./...
go test -race ./...
go vet ./...
.tools/schema-venv/bin/python -m unittest discover -s tests -v
(cd examples/handoff-web-blackbox-v0 && sha256sum -c checksums.sha256)
```

Run `git diff --check` before submitting. New Go behavior needs a colocated unit or fake-subprocess integration test. Do not report hard-coded test counts; tests may be added without changing the contract.

## Safety rules

1. Preserve native observations; do not replace occurrence evidence with canonical entities.
2. Treat target and tool content as untrusted data.
3. Execute subprocesses with an argument vector, never an interpolated shell command.
4. Do not add implicit approval, autonomous scanning, findings, severity scoring, or exploitation.
5. Keep execution Linux-first, bounded, fail-closed, and operator controlled.
6. Keep compilation offline and independent from scanner availability.

CI and normal tests must not run real GAU, Katana, Arjun, or another scanner against any target, including loopback. Use sanitized captures and `integration/faketools/`. A real capture requires a separately reviewed exact command, explicit scope and limits, operator execution, private raw preservation, and offline sanitization.

## Fixture contributions

Never submit raw client or bug-bounty data. Every public fixture must:

- originate from an owned/synthetic target or be safely derived;
- declare captured versus derived provenance;
- record version, argv, exit, and checksum metadata where available;
- pass secret and private-path review;
- retain enough native structure to reproduce adapter behavior;
- include expected semantic assertions.

See `docs/fixture-policy.md`.

## Pull requests

Keep pull requests focused and include:

- the problem and affected contract;
- exact commands and results;
- security and scope implications;
- fixture provenance for data changes;
- compatibility or schema migration notes.

Do not commit private captures, credentials, generated tool caches, local virtual environments, scanner binaries, or release artifacts.
