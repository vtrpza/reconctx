# Security Test Matrix

This matrix maps the fifteen threat classes in `docs/threat-model.md` to candidate evidence. “Automated” means a repository test or CI check exists; “manual” is an explicit release gate; “pending” blocks v0.1.0 until supplied. Test names are stable evidence—test counts are not.

| ID | Boundary | Automated evidence | Manual/release evidence | Status |
|---|---|---|---|---|
| T-01 | Shell, argument, and terminal injection | `TestPlanDisplayEscapesControlCharacters`; `TestRunnerRedactsSensitiveCommandAndEnvironmentArtifacts`; direct argv execution in runner tests | G4 reviews rendered argv and process command line | Automated + manual |
| T-02 | Approval bypass or stale approval | `TestRootRejectsImplicitApprovalFlag`; `TestTransitionRequiresExactCollectionApproval`; `TestTransitionRequiresFreshArjunApprovalAndSupportsSkip`; approval digest/state tests | G4 confirms zero child before both exact-digest approvals | Automated + manual |
| T-03 | Tool/path/version substitution | preflight resolution, writable-path, exact environment snapshot, isolated GAU config, non-executing metadata, approved-PATH, and unsupported-version tests; state-machine binary revalidation | G4 records resolved path, version, identity, hash, and effective config behavior | Automated + manual |
| T-04 | Scope or canonicalization bypass | evaluator strictness/normalization tests; `TestKatanaScopePatternDoesNotWidenURLPrefix`; URL vectors, ambiguity tests, Go/Python differential helper, and `FuzzCanonicalURL` | G4 uses owned loopback scope and observes no escape | Automated + manual |
| T-05 | Control-sequence and log deception | plan display escaping; runner redaction and bounded capture tests | Review raw versus display artifacts without executing content | Automated + manual |
| T-06 | Prompt injection through target/tool content | `TestCompileIsDeterministicChecksummedAndFailClosed` checks the handoff trust warning; compiler output keeps target data in factual records with Evidence references | G4 benchmark review treats all target text as data and exposes no execution interface | Automated + manual |
| T-07 | Secret or private-path leakage | runner sensitive command/environment redaction; integrity trust-boundary test; public fixture validation | G4/release secret and private-path review of handoff, logs, SBOM, and test log | Automated + manual |
| T-08 | Unsafe workspace filesystem objects | workspace traversal, symlink, hardlink, special-file, permissions, identity, atomic-write, finalized-artifact, `TestPublishTreeIsAtomicFinalAndIgnoresStaleStages`, and exact `ListTree` tests; `FuzzManagedIntegrityPaths`; unsafe runner output tests | Candidate is built from a clean private workspace | Automated + manual |
| T-09 | Resource exhaustion and unbounded work | adapter artifact bounds; runner output/record caps; plan/queue ceilings and strict scope | G4 confirms effective rates, concurrency, timeouts, queue cap, and disk use | Automated + manual |
| T-10 | Timeout, interruption, or orphaned descendants | runner timeout, cancellation, final-wait, stopped-process, new-session, descendant, and process-group tests | G4 checks process table, closed port, and persisted interruption state | Automated + manual |
| T-11 | Parser confusion and false completeness | all GAU/Katana/Arjun fixture tests; `FuzzNativeParsers`; runner incomplete-success rejection; failed/partial native artifact tests | Compatibility limitations and G4 semantic statuses are reviewed | Automated + manual |
| T-12 | Provenance or package tampering | model merge/locator tests; `TestCompileRevalidatesRawBytesAndLocators`; `TestCompileRejectsDanglingCandidateProvenance`; `TestBuildRejectsCandidateDecisionArtifactDrift`; pre-prompt/post-approval raw and wordlist revalidation tests; resume checksum, missing-mapping, extra-entry, and partial-final rejection; `TestIntegrityChecksumsAndTrustBoundaries` | G4 material claims are traced to Evidence IDs/native locators and release packages are independently checksum-verified | Automated + manual |
| T-13 | Fixture poisoning or unsafe publication data | Python schema/fixture/handoff validation; per-case and example SHA-256 manifests | Fixture provenance, sanitization, secrets, and private paths reviewed before G5 | Automated + manual |
| T-14 | Dependency, CI, and release supply chain | `go mod verify`, `go mod tidy -diff`, hash-locked Python install, pinned CI action commits, deterministic double build | Clean-candidate SBOM/license review, checksum and provenance verification | **Local SBOM; review pending** |
| T-15 | Agent/recon finding inflation | compiler factual-view test; candidate policy records factual reason/evidence; schemas contain no finding/severity record | G4 answers benchmark questions from Evidence and rejects unsupported vulnerability/severity claims | Automated + manual |

## Required fuzz gate

`FuzzCanonicalURL`, `FuzzNativeParsers`, and `FuzzManagedIntegrityPaths` cover URL canonicalization, native GAU/Katana/Arjun parsing, and managed/rooted path handling. Each target completed a 30-second local run on 2026-07-16. The exact clean candidate run still must be retained in `dist/release-test-results.txt` as specified in `docs/release-checklist.md`.

## Dynamic limitations

- Fake subprocess tests demonstrate containment mechanics; they cannot prove every kernel/tool interaction on every Linux host.
- Sanitized fixtures cover exact known native contracts, not arbitrary scanner versions or malformed-output space.
- Secret scanning reduces accidental disclosure risk but cannot prove the absence of all sensitive context; human review remains mandatory.
- Race tests cover exercised code paths, not semantic races in third-party tools.
- Deterministic build comparison is environment-specific until the released artifact is independently reproduced.
- Arjun preflight binds its entrypoint, version metadata, and effective environment, but not every interpreter/site-package byte; v0 requires a private immutable virtual environment through execution.

Any failed automated check, unresolved pending item, or unexpected G4 behavior reopens the corresponding gate and stops publication.
