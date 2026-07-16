# Repository Guidelines

## Project Structure & Module Organization

The production CLI is Go 1.24: `cmd/reconctx/` contains the entry point and `internal/` contains domain packages such as `app`, `approval`, `canonical`, `scope`, and `workspace`. Python under `reference/` provides deterministic executable specifications; repository-level Python tests live in `tests/`. Use `integration/faketools/` for subprocess tests. Versioned contracts and data live in `schemas/v0/`, `profiles/`, and `fixtures/`; `examples/handoff-web-blackbox-v0/` is an immutable sample handoff. Treat `spikes/` and `benchmarks/` as discovery material, not production modules.

## Build, Test, and Development Commands

- `go build -o build/reconctx ./cmd/reconctx` builds the local CLI.
- `go run ./cmd/reconctx --help` exercises the currently implemented command surface.
- `go fmt ./...` applies the standard Go formatter.
- `go test ./...` runs all packages in the main Go module.
- `.tools/schema-venv/bin/python -m unittest discover -s tests -v` runs schema, fixture, reference, and handoff validation. Install the pinned packages from `requirements-dev.lock` into that local environment first.
- From `examples/handoff-web-blackbox-v0/`, run `sha256sum -c checksums.sha256` to verify the materialized example.

## Coding Style & Naming Conventions

Follow `gofmt`; use lowercase Go package names, `PascalCase` for exported identifiers, and `camelCase` internally. Python uses four spaces, `snake_case` functions/modules, and `PascalCase` classes. Keep reference code deterministic and dependency-light. Preserve raw observations and provenance when normalizing data; discuss schema-breaking changes before implementation.

## Testing Guidelines

Develop behavior test-first. Go tests are colocated as `*_test.go` with `TestXxx`; Python uses `tests/test_*.py` and `unittest`. No numeric coverage threshold is defined, but every new Go behavior needs unit or integration coverage. Never run real GAU, Katana, Arjun, or other scanners in CI or normal tests; use sanitized captures or fake subprocesses.

## Commit & Pull Request Guidelines

History uses concise Conventional Commit subjects, for example `feat: add canonicalization primitives` and `chore: bootstrap Go CLI skeleton`. Keep PRs focused and state the problem and affected contract, exact test results, security/scope implications, fixture provenance, and any compatibility or schema migration notes.

## Security & Evidence Handling

Do not commit private captures, credentials, scanner binaries, caches, or virtual environments. Treat target and tool output as untrusted data. Active capture requires explicit operator approval, bounded scope and limits, private raw preservation, and offline sanitization before adding a public fixture.
