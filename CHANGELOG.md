# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

No changes recorded after the v0.1.0 release candidate.

## [0.1.0] - Unreleased

First Linux amd64 release candidate. Replace `Unreleased` with the publication date only after G5 approval.

### Added

- Operator-controlled `plan`, `run`, `resume`, and offline `build` workflow.
- Two exact SHA-256 digest approval gates for collection and Arjun execution.
- Strict scope, URL canonicalization, binary preflight, private workspace, and process-containment primitives.
- Fixture-backed GAU 2.2.4, Katana v1.6.1, and Arjun 2.2.7 adapters.
- Deterministic candidate selection, normalized evidence records, integrity checks, and portable handoff compilation.
- Sanitized normal and failure fixtures, executable reference specifications, and fake-subprocess tests.

### Security

- No non-interactive approval shortcut or agent-controlled active execution.
- Fail-closed behavior for plan drift, binary drift, unsafe paths, invalid native artifacts, and integrity failures.
- Bounded stdout, stderr, native artifacts, records, timeouts, and process groups.

### Known limitations

- Linux amd64 only.
- Only the exact scanner versions listed in `docs/compatibility-matrix-v0.md` are supported.
- The generated Arjun queue is approved as-is, skipped, or cancelled; v0.1.0 has no interactive candidate edit or reorder operation.
- `resume` continues only approval and compile checkpoints; unsafe in-flight and terminal states require a new reviewed plan.
- Authenticated HAR/Burp input, BBOT import, dashboards, daemons, distributed execution, and findings/severity generation are deferred.
- Publication remains blocked until the clean-candidate release checklist, operator loopback acceptance, release artifacts, SBOM review, and G5 approval pass.
