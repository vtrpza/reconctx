# Compatibility Matrix v0

**Status:** discovery evidence; production adapters are not implemented.

## Platform

| Component | Verified environment | MVP support decision |
|---|---|---|
| OS | Parrot/Linux amd64 | Linux-first |
| Production CLI | Go 1.24.4 | Go 1.24+; Linux-first |
| Reference tooling | Python 3.13.5 | development/fixtures only |
| Agent runtime integration | none | explicitly out of scope |

## Pinned implementation dependencies

| Dependency | Version | Purpose | License reviewed |
|---|---:|---|---|
| `golang.org/x/net` | 0.48.0 | UTS #46 / IDNA processing | BSD-3-Clause |
| `golang.org/x/text` | 0.32.0 | Unicode case mapping and NFC | BSD-3-Clause |
| `golang.org/x/sys` | 0.41.0 | Linux `openat2` and `renameat2` | BSD-3-Clause |
| `go.yaml.in/yaml/v3` | 3.0.4 | strict operator-facing YAML | MIT / Apache-2.0 |
| Python `idna` | 3.4 | Unicode 15 oracle used only by tests | BSD-3-Clause |

## Recon tools

| Tool | Captured version | Interface validated | Fixture status | Known constraints |
|---|---:|---|---|---|
| Katana | v1.6.1 | JSONL crawl, stdout/native equivalence, exact loopback scope, interrupted valid-prefix handling | normal + interrupted fixtures validated | build required Go toolchain switching to 1.25.12; interrupted output is partial coverage, never a complete-crawl claim |
| GAU | 2.2.4 | canonical text plus JSON regression | text and extensionless-drop fixtures validated | JSON mode drops extensionless URLs; output path appends; provider errors may exit 0; per-line provider unavailable |
| Arjun | 2.2.7 | GET, POST form, JSON, zero, interruption and request-timeout failure | six loopback fixtures validated | zero may omit native JSON; interrupted absence is unknown; request-timeout path raised internal `AttributeError`; `--version` prints help, so runtime banner/package metadata is required |

## Schema and package

| Contract | Version | State |
|---|---|---|
| Normalized records | `reconctx/v0` | Draft 2020-12 schemas and 46-test reference suite validated |
| URL canonicalization | `url-canonicalization/v0` | executable vectors plus direct Go↔Python differential gate validated |
| Agent view | `reconctx-agent-view/v0` | deterministic derived projection; non-authoritative |
| Handoff manifest | `reconctx/v0` | checksums and cross-references validated |
| BBOT importer | planned after first vertical slice | no runtime validation yet |

## Support meaning

“Validated” means demonstrated against the pinned sanitized fixtures and exact versions above. It does not claim compatibility with newer/older versions or every tool option. Unknown versions must be preflighted and should fail closed or be labeled unsupported until a fixture confirms their native contract.

Version expansion requires:

1. operator-captured private evidence;
2. sanitized/derived fixture review;
3. adapter regression tests;
4. matrix update;
5. conservative release notes describing dynamic limitations.
