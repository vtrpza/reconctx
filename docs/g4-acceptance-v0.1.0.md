# v0.1.0 G4 Operator Acceptance

- **Status:** accepted
- **Recorded at:** 2026-07-16T11:39:13Z
- **Operator and reviewer:** `xen0`
- **Operator attestation:** `G4 aceito por xen0`

## Candidate

- executable commit: `d15df5bf17c82b0f28ec8e0d4af06fbd0b345a14`;
- version: `v0.1.0`;
- Linux amd64 binary SHA-256: `20fdfc198d34ccbdaa59eb80f46779f9af23dc6ab7d7e0fc9730a3fca8d2f59d`;
- owned scope: `http://127.0.0.1:18081/` only;
- run: `run_713cbec6922ece2a134e64f0a1a42882`.

## Bound approvals

| Phase | Decision | Approved digest | Timestamp |
|---|---|---|---|
| Collection | `approve` | `sha256:b8a4f1da5d1d7e5a56fd4e86b540c387cc4ecd557b642d91b9ebe831f6ceb449` | `2026-07-16T11:30:24.658559271Z` |
| Arjun | `approve` | `sha256:2892889bcbe6480ed2bb3e6d70884ed9e92fc5da68f526f8a4a79b1a87f76fb2` | `2026-07-16T11:31:23.577061549Z` |

## Tool identities

| Tool | Version | Executable SHA-256 |
|---|---|---|
| GAU | `2.2.4` | `sha256:6f9710b828d1bde9d8d5515ffbf08d1986c20800f29eeae828dba2e1a804396c` |
| Katana | `v1.6.1` | `sha256:c691e82cec90ac570336a47cfbf4646c682b8569d9e94e6d1d04c2cf6392a374` |
| Arjun | `2.2.7` | `sha256:7e3940f706bc133f58d76b8e2a937fbd182a41a0bfab6221a65980531464b443` |

## Result and verification

- The final status was correctly `partial`: GAU was partial, Katana retained valid records before its outer timeout, and one of five approved Arjun executions timed out. No false completeness or absence claim was made.
- The handoff contains 1 asset, 6 endpoints, 3 parameters, 12 observations, 17 Evidence records, 15 relationships, and 7 tool executions. Six loopback HTTP observations returned status 200.
- The candidate policy recorded 6 decisions and included 5 bounded Arjun targets.
- All 32 checksum entries passed: 31 packaged payloads plus the manifest. The checksum manifest SHA-256 is `f3d1e3a233e6c460f61b2abbf8d39c24f6ee6e648813f7dc66199a6a4f7cef90`; the handoff manifest SHA-256 is `ec5525f31176620dccde2ed520be61be6546b39e64c7371634bb8500c2e1b8f8`.
- Independent validation accepted all 62 schema-valid records and resolved every record, artifact, and Evidence reference.
- Two additional offline builds were byte-identical to each other and to the original handoff, and their checksums passed.
- Review found no scope escape, private path, operator label, obvious secret, symlink, hardlink, or unlisted handoff entry.
- At final acceptance, no GAU, Katana, Arjun, or fixture process remained and the loopback port refused connections.
- Private plans, approvals, execution raws, and workspace state remain outside the repository.

This record closes G4 only. It does not authorize tagging, pushing, uploading, or publication. Any executable, dependency, scope, or approval-semantics change reopens G4.
