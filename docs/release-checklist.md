# v0.1.0 Release Checklist

This checklist produces a reviewable **local** Linux amd64 candidate. It does not authorize tagging, pushing, uploading, or publishing. Run it from a clean checkout at the exact candidate commit and save command output in `dist/release-test-results.txt`.

Local verification on 2026-07-16 passed the full Go test and race suites, `go vet`, the Python suite, fixture/example checksums, deterministic reference regeneration, and all three required 30-second fuzz runs. A reproducible dirty-tree review binary, checksum, SPDX SBOM, license inventory, and test log were also regenerated. [G4 operator acceptance](g4-acceptance-v0.1.0.md) is recorded; these results do not replace the clean-candidate rerun, independent artifact/SBOM review, or G5.

## 1. Preconditions and closed gates

- [x] G0 implementation authority is recorded.
- [ ] G1–G3 changes are code-reviewed and all automated checks below pass.
- [x] `requirements-dev.lock` contains verified wheel hashes and an offline installation succeeded with `--require-hashes`.
- [x] Required `Fuzz*` entry points cover canonicalization, native parsers, and managed/rooted path handling.
- [x] G4 operator acceptance is signed for one bounded owned-loopback run through both approval gates.
- [ ] The worktree contains no private captures, credentials, scanner binaries, caches, virtual environments, or unrelated generated files.
- [ ] `CHANGELOG.md`, `SECURITY.md`, the compatibility matrix, and dynamic limitations match the candidate.

Do not substitute existing sanitized fixtures for G4: the gate demonstrates the production CLI under direct operator control. Do not run an external-target acceptance test.

## 2. Source and dependency verification

```bash
git status --short
git diff --check
test -z "$(gofmt -l $(git ls-files '*.go'))"
go mod verify
go mod tidy -diff
```

- [ ] `git status --short` is empty at the candidate commit.
- [ ] Every tracked Go source file is formatted with `gofmt`.
- [ ] The Go module graph and sums verify without modification.
- [x] Python installation uses the hash-locked development requirements.
- [ ] Direct and transitive dependency licenses are recorded in `dist/THIRD_PARTY_LICENSES.txt` and reviewed for Apache-2.0 distribution.
- [ ] CI action references are immutable full commit SHAs.

The repository intentionally uses one non-publishing CI workflow and no GoReleaser configuration.

## 3. Automated gates

```bash
go test ./...
go test -race ./...
go vet ./...
.tools/schema-venv/bin/python -m unittest discover -s tests -v
```

Run each available security fuzz target for at least 30 seconds. Package names may be split if more than one `Fuzz*` function exists:

```bash
go test ./internal/canonical -run '^$' -fuzz '^Fuzz' -fuzztime=30s
go test ./internal/adapter -run '^$' -fuzz '^Fuzz' -fuzztime=30s
go test ./internal/workspace -run '^$' -fuzz '^Fuzz' -fuzztime=30s
```

Verify every tracked fixture manifest and the example handoff from the directory containing it:

```bash
find fixtures/cases -name checksums.sha256 -print0 | sort -z | while IFS= read -r -d '' manifest; do
  (cd "$(dirname "$manifest")" && sha256sum -c checksums.sha256)
done
(cd examples/handoff-web-blackbox-v0 && sha256sum -c checksums.sha256)
```

Regenerate the reference handoff and require no tracked difference:

```bash
.tools/schema-venv/bin/python -m reference.build_example_v0
git diff --exit-code -- examples/handoff-web-blackbox-v0
```

- [x] All automated commands pass in the current local verification; rerun and record them at the exact clean candidate commit.
- [x] No scanner executable is invoked by tests, fuzzing, reference generation, or CI.
- [x] Each required fuzz target completed a 30-second local run.
- [x] The T-01–T-15 mapping in `docs/security-test-matrix.md` is current.

## 4. Operator G4 acceptance

The operator, not an agent or CI job, performs this gate against the owned loopback fixture target:

1. Review the exact plan: tool paths, versions, hashes, scope, arguments, limits, environment, and outputs.
2. Enter the full collection digest; confirm no child exists before approval and no scope escape occurs after it.
3. Review the normalized candidate queue and its full digest.
4. Approve or skip the exact queue according to the test record; cancellation alone does not demonstrate the full approved path.
5. Interrupt one bounded run only if current process-control evidence requires it.
6. Confirm descendants are gone, the loopback port is closed, partial semantics are correct, and private raws remain private.
7. Build twice offline and verify deterministic, schema-valid, checksummed handoffs whose material claims resolve to Evidence IDs.

- [x] The [signed record](g4-acceptance-v0.1.0.md) includes candidate commit, binary SHA-256, tool identities, plan/queue digests, exact decisions, timestamps, results, and reviewer.

## 5. Reproducible Linux amd64 artifact

```bash
mkdir -p dist/build-a dist/build-b
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags='-s -w -X github.com/vtrpza/reconctx/internal/version.Version=v0.1.0' \
  -o dist/build-a/reconctx_0.1.0_linux_amd64 ./cmd/reconctx
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags='-s -w -X github.com/vtrpza/reconctx/internal/version.Version=v0.1.0' \
  -o dist/build-b/reconctx_0.1.0_linux_amd64 ./cmd/reconctx
cmp dist/build-a/reconctx_0.1.0_linux_amd64 dist/build-b/reconctx_0.1.0_linux_amd64
cp dist/build-a/reconctx_0.1.0_linux_amd64 dist/reconctx_0.1.0_linux_amd64
dist/reconctx_0.1.0_linux_amd64 --version
```

- [ ] Both builds are byte-identical.
- [ ] The binary reports exactly `v0.1.0` and runs on the clean Linux amd64 acceptance environment.
- [ ] `CGO_ENABLED=0` is preserved; a dependency problem reopens the dependency decision.

Required local release files:

```text
dist/reconctx_0.1.0_linux_amd64
dist/reconctx_0.1.0_checksums.txt
dist/reconctx_0.1.0_linux_amd64.spdx.json
dist/THIRD_PARTY_LICENSES.txt
dist/release-test-results.txt
dist/LICENSE
```

Generate an SPDX JSON SBOM with a pinned Syft release and record the tool/version in the test log. If automation is later added, the reviewed baseline pins are `anchore/sbom-action` v0.24.0 at `e22c389904149dbc22b58101806040fa8d37a610` and `actions/attest` v4.1.1 at `a1948c3f048ba23858d222213b7c278aabede763`; neither is needed in ordinary CI.

After the SBOM, license inventory, and test log are final, checksum every published artifact except the checksum file itself:

```bash
(cd dist && sha256sum \
  reconctx_0.1.0_linux_amd64 \
  reconctx_0.1.0_linux_amd64.spdx.json \
  THIRD_PARTY_LICENSES.txt \
  release-test-results.txt \
  LICENSE > reconctx_0.1.0_checksums.txt)
(cd dist && sha256sum -c reconctx_0.1.0_checksums.txt)
```

- [ ] SBOM contents match the candidate binary/module graph.
- [ ] Checksums, SBOM, licenses, and test log contain no secrets or private paths.
- [ ] A second reviewer verifies every artifact from its checksum.

## 6. G5 publication stop

- [ ] Fix the v0.1.0 changelog date before building the exact final artifact set; rebuild if it changes.
- [ ] Record the candidate commit and hashes in the approval request.
- [ ] Obtain explicit approval for this exact source tree and artifact set.
- [ ] Create an immutable `v0.1.0` tag only after approval.
- [ ] Publish the binary, checksum, SPDX SBOM, license inventory, project license, and test results together.
- [ ] Create provenance/attestation only for the exact published digest and verify it after publication.
- [ ] Re-download and verify the public artifacts from a clean environment.

Until every G5 item is approved, do not call `gh`, create or push a tag, upload artifacts, or publish a release.
