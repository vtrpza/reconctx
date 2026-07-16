# Approved Product Decisions v0

**Approved by:** operator  
**Approved at:** `2026-07-12T22:53:28-03:00`  
**Implementation approved at:** `2026-07-13T12:50:05-03:00`  
**Artifact publication approval:** v0.1.1 publication granted on 2026-07-16; repeat G4/G5 explicitly waived
**Status:** binding implementation inputs; changes require an explicit operator decision

## Product posture

- The product is an independent, compiler-first CLI.
- The first-party runner is intentionally minimal and supports the initial vertical slice.
- BBOT interoperability is provided through an importer after the first vertical slice.
- A pivot to a BBOT companion is allowed only if the competitive benchmark demonstrates at least 80% coverage of the acceptance story, including operator control, provenance, and handoff quality—not merely recon breadth.

## Stack policy

- Go is the default production implementation language.
- Python remains valid for fixtures, executable specifications, discovery scripts, and benchmark tooling.
- The ADR may override Go only if a material blocker is demonstrated by equivalent timeboxed spikes.

## Initial vertical slice

- GAU + Katana initial collection phase.
- Normalization, evidence preservation, candidate policy, and handoff compilation.
- Arjun parameter-discovery phase after separate approval.
- HAR follows the first vertical slice.
- BBOT import follows HAR/interoperability work.

## Approval UX

Two mandatory phase gates:

1. approve scope, effective GAU/Katana commands, limits, and output paths;
2. approve the exact Arjun candidate queue, methods, locations, limits, and effective commands.

Any plan drift requires renewed approval, including a new target/tool, scope expansion, increased intensity, changed effective command, or candidate count above the approved ceiling.

## Raw and handoff policy

- The private workspace preserves bounded raw captures; any capture-limit truncation is explicit and forces partial coverage.
- The handoff embeds only admitted, evidence-referenced artifacts after secret and private-path checks, or omits raw bytes.
- v0.1.1 has no unredacted/full-raw inclusion option.
- The manifest declares `embedded_sanitized`, `referenced`, or `omitted` raw policy.

## Active execution boundary

- The operator executes every active scanner/tool command.
- Hermes may install/configure tools, prepare targets, command previews, manifests, expected outputs, and validation scripts.
- Hermes does not execute Katana, Arjun, BBOT, reconFTW, proxies, or other active scanners, including against loopback.
- Passive external GAU queries also remain operator-executed.
- Offline parsers, fake subprocesses, malformed fixtures, schema tests, benchmarks, and static/source analysis may be executed autonomously.

## Failure-path policy

- Synthetic/fake cases cover tool missing, unsupported version, malformed/truncated data, ANSI/control output, oversized artifacts, unsafe paths/symlinks, cancellation plumbing, and cleanup.
- Real interrupted/timeout behavior for Katana and Arjun may be captured locally only through operator-executed commands after a command preview.
- No new external GAU capture is required by default.

## Platform and integration

- MVP is Linux-first while preserving portable schemas and avoiding deliberate cross-platform blockers.
- Pentest harness integration is an optional profile/export.
- The product never generates findings or severity automatically.

## Open-source policy

- License: Apache-2.0.
- Repository owner when publication is approved: `vtrpza`.
- Working name remains `reconctx` until availability/trademark review.
- No publication occurs before final review of README, LICENSE, SECURITY, threat model, fixture policy, and sanitized repository contents.

## Final implementation gate

**Status:** closed by explicit operator approval. Implementation and publication may proceed only through the safety/review gates in `.hermes/plans/2026-07-13_002250-reconctx-mvp-implementation.md`.

Discovery gates may run autonomously within the boundaries above. Production runner implementation requires a final explicit operator approval after:

- pipeline/approval contract;
- agent handoff benchmark;
- threat model;
- stack ADR;
- independent-vs-companion recommendation;
- remaining fixture-gap disposition;
- implementation plan.
