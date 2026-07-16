# reconctx

> Evidence, not terminal noise.

`reconctx` is an operator-controlled reconnaissance evidence compiler. It supervises bounded GAU, Katana, and Arjun runs, preserves native evidence, normalizes observations without erasing provenance, and builds portable handoffs for downstream analysis.

## Release status

The first release target is **v0.1.0 for Linux amd64**. The implementation is being prepared as a local release candidate; publication waits for every applicable pre-publication item in the [release checklist](docs/release-checklist.md) and explicit approval of the exact artifact set.

The supported external tool contracts are pinned to:

- GAU 2.2.4;
- Katana v1.6.1;
- Arjun 2.2.7.

These tools are not bundled. Other versions are unsupported until their native output and failure behavior are covered by fixtures.

## Workflow

```text
plan -> decision A -> bounded GAU/Katana capture
     -> offline normalization -> deterministic Arjun queue
     -> decision B -> bounded Arjun capture or explicit skip
     -> offline build -> CONTEXT.md + normalized records + Evidence map
```

Both active-phase decisions are bound to the full `sha256:<64 lowercase hex characters>` digest displayed by the CLI. There is no implicit or non-interactive approval flag. Any behavior drift invalidates an approval.

## Quick start

Build the local CLI:

```bash
go build -o build/reconctx ./cmd/reconctx
build/reconctx --version
```

Create a strict scope document, then render a plan:

```bash
build/reconctx plan \
  --target fixture.test \
  --seed http://fixture.test:18080/ \
  --scope scope.yaml \
  --wordlist /absolute/private/params.txt \
  --profile web-blackbox \
  --workspace /absolute/private/reconctx-work \
  --out run-plan.json
```

Run, resume, and compile only after reviewing the rendered paths, versions, hashes, arguments, scope, limits, and output roots:

```bash
build/reconctx run /absolute/private/reconctx-work/run-plan.json
build/reconctx resume --workspace /absolute/private/reconctx-work RUN_ID
build/reconctx build --workspace /absolute/private/reconctx-work --run RUN_ID --out handoff/RUN_ID
```

`plan` performs non-executing, bounded version-metadata preflight. `run` and `resume` can create approved network traffic; `resume` only continues approval checkpoints and rejects unsafe in-flight or terminal states. `build` is offline and never launches scanners. See the complete [CLI contract](docs/cli.md).

## Evidence model

`reconctx` keeps three layers separate:

1. native artifacts and immutable execution evidence;
2. normalized entities, observations, and relationships;
3. derived compact views for agents and humans.

Every material handoff claim should resolve to an Evidence ID and native locator. Historical URLs are not presented as currently observed. Parameter candidates are not findings. Missing and partial coverage is explicit.

## Safety boundaries

- CI and normal tests never run GAU, Katana, Arjun, or another scanner.
- Active execution requires a reviewed plan, bounded scope and limits, an interactive terminal, and an exact digest approval.
- Target and tool output is untrusted data, never instructions.
- Private raw evidence stays private and immutable; public fixtures are separately sanitized.
- The agent and offline compiler have no active execution path.

See [SECURITY.md](SECURITY.md), [CONTRIBUTING.md](CONTRIBUTING.md), and the [fixture policy](docs/fixture-policy.md).

## Validation

```bash
go test ./...
go test -race ./...
go vet ./...
.tools/schema-venv/bin/python -m unittest discover -s tests -v
(cd examples/handoff-web-blackbox-v0 && sha256sum -c checksums.sha256)
```

The repository CI also verifies fixture checksums, deterministic reference regeneration, module tidiness, and two byte-identical static Linux amd64 builds.

## Scope of v0.1.0

The first release deliberately excludes autonomous scanning, exploitation, finding/severity generation, authenticated HAR/Burp workflows, BBOT import, dashboards, daemons, distributed execution, and non-Linux process semantics.

Discovery contracts and design evidence remain available under `docs/`, `benchmarks/`, `fixtures/`, and `examples/`. Current implementation and gate status is recorded in [docs/discovery-status.md](docs/discovery-status.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
