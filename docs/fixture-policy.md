# Public Fixture Policy

## Goal

Fixtures make third-party tool behavior reproducible without running active tools in CI or publishing private engagement data.

## Data classes

| Class | Location | Publishable | Rules |
|---|---|---:|---|
| Private capture | `private-captures/` | no | immutable original; restricted access; checksummed |
| Sanitized captured fixture | `fixtures/cases/` | yes after review | copied from private capture; provenance `captured` or explicit `copied-from-loopback-capture`; sensitive values replaced |
| Derived regression fixture | `fixtures/cases/` | yes after review | provenance `derived`; identifies source case; minimal mutation documented |
| Synthetic generated fixture | test temp directory or fixture tree | yes | deterministic generator and expected semantics |
| Handoff example | `examples/` | yes after review | only sanitized/derived inputs; manifest/checksums required |

## Capture requirements

A capture manifest should retain:

- case and tool version;
- argv and redacted display command;
- target class, never unnecessary target identity;
- start/end/duration and exit code;
- interruption/timeout state;
- stdout, stderr and native output presence;
- environment metadata without environment secrets;
- checksums and sanitization status.

Private raw is never modified in place. Sanitization creates a new tree.

## Sanitization

Review for:

- authorization/cookie/header values;
- credentials, tokens, API keys and session identifiers;
- client/customer names, domains, IPs and internal paths;
- local usernames/home paths;
- request/response bodies containing personal or business data;
- callback URLs and external infrastructure;
- terminal control sequences and binary payloads.

Use deterministic placeholders that preserve parser shape and length characteristics where relevant. Record every semantic replacement in the manifest. Do not claim a fixture is captured if its behavior was materially synthesized.

## Quality gates

Before publication:

1. verify all checksums;
2. validate manifest and normalized records against pinned schemas;
3. run structural/semantic expected-output tests;
4. scan for sensitive strings and private paths;
5. confirm no symlink, FIFO, device or path traversal entry exists;
6. verify public files resolve only within the fixture root;
7. confirm the fixture does not authorize or trigger network activity;
8. review by someone other than the capture author when practical.

## Immutability and updates

Published fixture bytes are immutable within a case version. If sanitization, metadata or expected semantics change:

- create a new case/version or explicitly document a derived fixture;
- retain the old regression input when compatibility testing needs it;
- never silently rewrite evidence while keeping its checksum/ID.

## CI policy

CI may parse, normalize, compile, validate and fuzz fixtures. It must not execute real reconnaissance tools against network targets. Fake subprocesses are allowed when they cannot access the network and are bounded by timeout/process-group cleanup.

## Prohibited content

Never publish:

- raw client/engagement artifacts;
- live credentials or tokens;
- non-public vulnerability details unrelated to reconctx;
- third-party data without redistribution rights;
- scanner outputs whose license prohibits redistribution;
- real target identifiers when synthetic substitution is sufficient.
