# Compatibility Matrix v0

**Status:** v0.1.1 candidate contract; publication requires fresh acceptance. Compatibility means fixture-backed support for the exact versions below, not a promise for adjacent versions or every tool option.

## Platform

| Component | Verified version/environment | v0.1.1 support |
|---|---|---|
| Operating system | Linux amd64 | Supported release target |
| Production CLI | Go 1.24.4 | Built as a static Linux amd64 binary |
| Reference tooling | Python 3.13.5 | Development and fixture validation only |
| macOS, Windows, other architectures | Not accepted | Unsupported in v0.1.1 |
| Agent runtime integration | None | Explicitly out of scope |

## Implementation dependencies

| Dependency | Pinned version | Purpose | License |
|---|---:|---|---|
| `github.com/google/jsonschema-go` | 0.4.3 | Draft 2020-12 validation of emitted records, candidate decisions, and manifests | MIT |
| `golang.org/x/net` | 0.48.0 | UTS #46 / IDNA processing | BSD-3-Clause |
| `golang.org/x/text` | 0.32.0 | Unicode case mapping and NFC | BSD-3-Clause |
| `golang.org/x/sys` | 0.41.0 | Linux `openat2` and `renameat2` | BSD-3-Clause |
| `go.yaml.in/yaml/v3` | 3.0.4 | Strict operator-facing YAML | MIT / Apache-2.0 |
| Python `idna` | 3.4 | Unicode oracle used only by tests | BSD-3-Clause |

The complete dependency and license inventory must be regenerated and reviewed for each release candidate. Python packages are pinned by version and verified wheel SHA-256.

## Reconnaissance tools

| Tool | Supported version | Validated interface | Conservative limitations |
|---|---:|---|---|
| GAU | 2.2.4 | Canonical text output; JSON regression; provider diagnostics | JSON mode can drop extensionless URLs; output paths append; provider errors can exit 0; provider identity is unavailable per native line |
| Katana | v1.6.1 | JSONL crawl; normal and interrupted valid-prefix fixtures | Interrupted output is partial coverage; URL-prefix scope conservatively excludes percent-escaped discovered suffixes |
| Arjun | 2.2.7 | GET, POST form, JSON, zero-result, interruption, and request-timeout fixtures | Zero may omit native JSON; interrupted absence remains unknown; request timeout may surface an internal tool error |

Tool binaries are not bundled. Preflight checks the resolved executable, version metadata, file identity, and SHA-256 without starting the tool. GAU and Katana versions come from Go build information with the exact expected main-module path. Tools receive an approved empty private run directory as `HOME`, never the operator's ambient home; GAU also uses an approved deliberately absent config path inside its new execution directory. Arjun 2.2.7 uses bounded `METADATA` from the `arjun` distribution in the same environment prefix as its absolute Python entrypoint/interpreter; an unbound `/usr/bin/env python` entrypoint is rejected. Hash-bound wrapper shims may carry the deterministic `reconctx-tool-metadata/v0` marker used by integration fake tools. A changed executable or unsupported version fails closed before active execution.

The v0 plan binds the Arjun entrypoint identity, version, and effective environment, but not a digest of every interpreter and installed package file in its virtual environment. Operators must therefore use a private, immutable Arjun environment between planning and execution; full Python environment/`RECORD` closure hashing is deferred until real acceptance evidence shows it is needed.

## Timeout compatibility

v0.1.1 keeps the scanner request-timeout interface unchanged: GAU `--timeout 45`, Katana `-timeout 10`, and Arjun `-T 15`. It separately binds the runner's total wall-clock execution ceiling: 900 seconds for GAU, 7,200 seconds for each Katana seed, and 7,200 seconds for each Arjun candidate. Both values are approval- and digest-bearing limits.

v0.1.0 plans contain only `timeout_seconds`. v0.1.1 does not reinterpret that request timeout as an execution deadline or silently supply a new ceiling; such plans fail closed and must be regenerated and approved.

## Data contracts

| Contract | Version | State |
|---|---|---|
| Normalized records | `reconctx/v0` | Implemented and schema/fixture tested |
| Plan | `reconctx-plan/v0` | v0.1.1 requires separate request and execution timeouts; older plans fail closed |
| URL canonicalization | `url-canonicalization/v0` | Go implementation checked against executable vectors and Python oracle |
| Candidate queue | `reconctx-candidate-queue/v0` | Deterministic, capped, and digest-bound, including both Arjun timeout limits |
| Agent view | `reconctx-agent-view/v0` | Deterministic derived projection; non-authoritative |
| Handoff manifest | `reconctx/v0` | Checksummed and cross-reference validated |
| BBOT importer | Deferred | No runtime support in v0.1.1 |

## Expanding support

Supporting a new platform or scanner version requires:

1. operator-captured private evidence from an authorized target;
2. sanitized or derived fixture review;
3. adapter and failure-path regression tests;
4. process-control acceptance where behavior changes;
5. matrix and release-note updates describing dynamic limitations.
