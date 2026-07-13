# Capture Command Preview — Gate Before Active Execution

**Status:** reviewed, authorized and executed for the listed loopback cases  
**Target local:** `http://127.0.0.1:18080`  
**Tool wrappers:** `./tools/bin/{katana,gau,arjun}`

## Verified toolchain

| Tool | Version | Isolation |
|---|---:|---|
| Katana | v1.6.1 | `[LOCAL_GO_BIN]/katana` via local wrapper |
| GAU | 2.2.4 | `[LOCAL_GO_BIN]/gau` via local wrapper |
| Arjun | 2.2.7 | project venv; wrapper removes inherited `PYTHONPATH` |

## Start local fixture target

```bash
python3 -m fixture_target.app --host 127.0.0.1 --port 18080
```

Preflight:

```bash
curl --fail --silent --show-error http://127.0.0.1:18080/healthz
ss -ltnp 'sport = :18080'
```

Expected socket: `127.0.0.1:18080`, never `0.0.0.0`.

## KAT-NORMAL-MINIMAL

Activity:

- target: loopback only;
- depth: 2;
- rate: 2 requests/second;
- concurrency: 1;
- parallelism: 1;
- timeout: 10 seconds;
- raw request/response and body omitted from JSONL.

Command preview:

```bash
./tools/bin/katana \
  -u 'http://127.0.0.1:18080/' \
  -cs '^http://127\.0\.0\.1:18080(?:/|$)' \
  -d 2 \
  -j -nc -silent \
  -rl 2 -c 1 -p 1 -timeout 10 \
  -or -ob \
  -o 'private-captures/katana/v1.6.1/KAT-NORMAL-MINIMAL/native-output.jsonl'
```

Expected capture directory:

```text
private-captures/katana/v1.6.1/KAT-NORMAL-MINIMAL/
├── manifest.json
├── command.txt
├── version.txt
├── stdout.raw
├── stderr.raw
├── native-output.jsonl
├── exit-code.txt
├── environment.json
└── checksums.sha256
```

## ARJUN-GET-FOUND

Activity:

- target: `/api/search` on loopback;
- method: GET;
- wordlist: 9 deterministic entries;
- rate: 1 request/second;
- threads: 1;
- timeout: 15 seconds.

Command preview:

```bash
./tools/bin/arjun \
  -u 'http://127.0.0.1:18080/api/search' \
  -m GET \
  -w 'fixtures/shared/arjun-minimal.txt' \
  -t 1 --rate-limit 1 -T 15 \
  -oJ 'private-captures/arjun/2.2.7/ARJUN-GET-FOUND/native-output.json'
```

Target ground truth: `q`, `debug`. Observed Arjun detection: `q`; `debug` was a false negative.

## ARJUN-POST-FORM-FOUND

```bash
./tools/bin/arjun \
  -u 'http://127.0.0.1:18080/api/update' \
  -m POST \
  -w 'fixtures/shared/arjun-minimal.txt' \
  -t 1 --rate-limit 1 -T 15 \
  -oJ 'private-captures/arjun/2.2.7/ARJUN-POST-FORM-FOUND/native-output.json'
```

Target ground truth and observed Arjun detection: `id`, `name`.

## ARJUN-JSON-FOUND

```bash
./tools/bin/arjun \
  -u 'http://127.0.0.1:18080/api/json' \
  -m JSON \
  -w 'fixtures/shared/arjun-minimal.txt' \
  -t 1 --rate-limit 1 -T 15 \
  -oJ 'private-captures/arjun/2.2.7/ARJUN-JSON-FOUND/native-output.json'
```

Target ground truth: `id`, `filter`, `debug`. Observed Arjun detection: `filter`, `id`; `debug` was a false negative.

## ARJUN-ZERO

```bash
./tools/bin/arjun \
  -u 'http://127.0.0.1:18080/api/no-params' \
  -m GET \
  -w 'fixtures/shared/arjun-minimal.txt' \
  -t 1 --rate-limit 1 -T 15 \
  -oJ 'private-captures/arjun/2.2.7/ARJUN-ZERO/native-output.json'
```

Target ground truth and observed Arjun detection: none. Arjun exited `0` and did not create the requested `-oJ` file.

## GAU-LIVE-OWNED-DOMAIN

Required input:

```text
FIXTURE_ARCHIVE_DOMAIN=<operator-owned or explicitly authorized domain>
```

### GAU 2.2.4 output contract

Do **not** use `--json` for the canonical fixture from release 2.2.4. Its JSON writer adds an empty extension to the blacklist and then drops every extensionless URL. This was reproduced with three provider records and is fixed upstream after the release, but remains present in the `v2.2.4` binary.

Canonical release fixture uses native line-oriented text:

```bash
./tools/bin/gau "$FIXTURE_ARCHIVE_DOMAIN" \
  --verbose \
  --providers otx,urlscan \
  --threads 1 \
  --timeout 45 \
  --o 'private-captures/gau/2.2.4/GAU-APEX-SUBS-TEXT/native-output.txt'
```

Add `--subs` only when subdomains are explicitly in scope. Capture Wayback and Common Crawl in separate cases because either source may time out/fail independently, while GAU still exits `0`.

Preserve the empty `--json` output as a regression fixture named `GAU-JSON-EXTENSIONLESS-DROP`.

## Stop gate

The operator authorized Katana and Arjun only against `127.0.0.1:18080`. The listed local cases completed and the listener was shut down. This authorization does not extend to new cases, changed budgets or external targets. Any failure-path expansion requires a new command preview before execution.
