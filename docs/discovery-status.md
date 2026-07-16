# Discovery and Release Status

**Observed:** 2026-07-16

**Target:** v0.1.0, Linux amd64

**Publication:** not performed; G5 remains closed

The discovery contracts have moved into a production Go implementation. The remaining work is verification and release gating, not architecture discovery.

## Gate status

| Gate | Required evidence | Current status |
|---|---|---|
| G0 — implementation authority | Explicit approval of the implementation plan | Passed |
| G1 — foundations | CLI skeleton, canonical JSON/URL, strict scope, private workspace, preflight, digest approvals | Implemented; full local automated verification passed |
| G2 — supervised execution | Fake-tool process containment, immutable capture, bounded output, timeout/interruption semantics | Implemented; full local test/race/vet verification passed |
| G3 — adapters and compiler | All public tool fixtures, deterministic queue, integrity checks, portable deterministic handoff | Implemented; full local Python/checksum/reference/fuzz verification passed |
| G4 — operator acceptance | One explicitly approved bounded loopback run through both approval gates | Pending operator execution |
| G5 — release/publication | Reproducible artifacts, hashes, SBOM/licenses, security matrix, clean-tree review, explicit publication approval | Dirty-tree review artifacts generated; clean candidate, independent review, and publication approval remain pending |

## Implemented release surface

- Linux-first Go CLI with `plan`, `run`, `resume`, `build`, and `--version` contracts;
- exact binary/version preflight for GAU 2.2.4, Katana v1.6.1, and Arjun 2.2.7;
- strict scope evaluation and deterministic URL canonicalization;
- digest-bound collection decisions (`approve`/`cancel`) and Arjun decisions (`approve`/`skip`/`cancel`) with no implicit approval flag or queue editing;
- fail-closed resume for approval and compile checkpoints, with no implicit retry of in-flight or terminal states;
- private rooted workspaces, atomic writes, and immutable finalized artifacts;
- process-group containment, bounded output, cancellation, timeout, and partial coverage handling;
- conservative adapters for every tracked normal and failure fixture;
- deterministic Arjun candidate policy and capped queue;
- deterministic compiler, Evidence locators, manifest/checksum validation, and no findings generation.

## Fixture evidence

Tracked sanitized fixtures cover:

- GAU canonical text, duplicate preservation, extensionless JSON loss, and provider-error behavior;
- Katana normal JSONL plus interrupted valid-prefix output;
- Arjun GET, POST form, JSON, zero-result, interrupted, and request-timeout behavior;
- the deterministic `examples/handoff-web-blackbox-v0/` handoff.

No external scan is authorized by the release work. CI and normal tests use fixtures and fake subprocesses only.

## Remaining first-release gates

1. Repeat and retain the green automated verification in the release test log at the exact clean candidate commit.
2. Have the operator execute and sign G4 against the owned loopback fixture target only.
3. Regenerate the Linux amd64 binary, checksum, SPDX SBOM, third-party license inventory, and test log from the exact clean candidate, then obtain independent review.
4. Review the sanitized tree and dynamic limitations, then obtain explicit artifact-specific G5 publication approval.

Historical discovery evidence remains in `docs/product-contract.md`, `docs/product-decisions-v0.md`, `docs/pipeline-v0.md`, `docs/threat-model.md`, `docs/tool-contract-matrix-v0.md`, `docs/failure-path-capture-preview-v0.md`, and `benchmarks/`. Test counts and timestamps in historical captures describe those captures only; current gates are command- and artifact-based.
