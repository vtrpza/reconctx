# Security Policy

## Project status

`reconctx` is in pre-implementation discovery. No production release is currently supported.

## Reporting a vulnerability

When the public repository is available, use GitHub private vulnerability reporting:

<https://github.com/vtrpza/reconctx/security/advisories/new>

Do not include exploit details, private targets, credentials, tokens, cookies, or unsanitized scanner output in a public issue.

If private reporting is temporarily unavailable, open a minimal public issue requesting a private maintainer contact. Do not disclose technical details until a private channel is established.

A useful report includes:

- affected commit/version and platform;
- minimal reproducible input using synthetic or sanitized data;
- expected versus observed security boundary;
- impact and prerequisites;
- whether any secret, target data, or external system was involved.

## Priority security boundaries

Reports are especially valuable when they concern:

- approval or scope bypass;
- command/argument/environment injection;
- path traversal, unsafe symlink following or special-file handling;
- secret leakage into handoffs, logs or fixtures;
- Evidence/manifest/checksum tampering;
- process-group cleanup failures or orphaned scanners;
- target content being interpreted as agent instructions;
- accidental active execution during dry-run, compilation or CI.

## Out of scope

- vulnerabilities in reconnaissance targets;
- findings produced by third-party tools;
- upstream scanner vulnerabilities that do not arise from reconctx integration;
- active testing of systems without authorization;
- reports containing real credentials or unnecessary private evidence.

Report third-party scanner issues to the corresponding upstream project. A reconctx report remains appropriate when its wrapper, adapter or evidence handling introduces an additional vulnerability.

## Coordinated disclosure

Maintainers will acknowledge a private report, reproduce it using bounded synthetic fixtures where possible, determine affected versions and coordinate a fix/advisory. Publication timing will be agreed with the reporter when practical.

No activity is authorized merely by this policy. Testing must remain within systems and data the reporter owns or is explicitly authorized to assess.
