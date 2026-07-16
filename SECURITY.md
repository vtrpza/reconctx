# Security Policy

## Supported versions

| Version | Platform | Security support |
|---|---|---|
| v0.1.x | Linux amd64 | Supported after publication |
| Earlier development snapshots | Any | Unsupported |

v0.1.0 is currently a local release candidate. Until it is published, reports should identify the affected commit rather than assume a supported binary.

## Reporting a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/vtrpza/reconctx/security/advisories/new). Do not include exploit details, private targets, credentials, tokens, cookies, or unsanitized scanner output in a public issue.

If private reporting is unavailable, open a minimal public issue requesting a private maintainer contact. Do not disclose technical details until a private channel exists.

A useful report includes:

- affected commit or version and platform;
- a minimal reproduction using synthetic or sanitized data;
- the expected and observed security boundary;
- impact and prerequisites;
- whether any secret, target data, or external system was involved.

## Priority boundaries

Reports are especially valuable when they concern:

- approval, scope, or limit bypass;
- command, argument, path, or environment injection;
- symlink, hardlink, traversal, or special-file handling;
- secret leakage into handoffs, logs, or fixtures;
- Evidence, manifest, or checksum tampering;
- process-group cleanup failures or orphaned scanners;
- target content being interpreted as instructions;
- active execution during planning, compilation, or CI.

## Out of scope

- vulnerabilities in reconnaissance targets;
- findings produced by third-party tools;
- upstream scanner vulnerabilities that do not arise from the integration;
- active testing of systems without authorization;
- reports containing real credentials or unnecessary private evidence.

Report third-party scanner issues upstream. A `reconctx` report is still appropriate when its wrapper, adapter, approval flow, or evidence handling creates an additional vulnerability.

## Coordinated disclosure

Maintainers will acknowledge a private report, reproduce it with bounded synthetic fixtures where possible, determine affected versions, and coordinate a fix and advisory. Publication timing will be agreed with the reporter when practical.

This policy grants no authorization to test third-party systems. Testing must remain within systems and data the reporter owns or is explicitly authorized to assess.
