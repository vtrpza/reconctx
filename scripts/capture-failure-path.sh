#!/usr/bin/env bash
set -Eeuo pipefail

terminate_pid_bounded() {
    local pid=$1
    local state=""
    local _

    if ! kill -0 "$pid" 2>/dev/null; then
        wait "$pid" 2>/dev/null || true
        return
    fi

    kill -TERM "$pid" 2>/dev/null || true
    kill -CONT "$pid" 2>/dev/null || true
    for _ in $(seq 1 30); do
        if ! kill -0 "$pid" 2>/dev/null; then
            break
        fi
        state=$(ps -o stat= -p "$pid" 2>/dev/null || true)
        [[ "$state" == Z* ]] && break
        sleep 0.1
    done
    if kill -0 "$pid" 2>/dev/null; then
        state=$(ps -o stat= -p "$pid" 2>/dev/null || true)
        if [[ "$state" != Z* ]]; then
            kill -KILL "$pid" 2>/dev/null || true
        fi
    fi
    wait "$pid" 2>/dev/null || true
}

extract_tool_version() {
    python3 - "$@" <<'PY'
import re
import sys

text = "\n".join(
    open(path, encoding="utf-8", errors="replace").read()
    for path in sys.argv[1:]
)
text = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", text)
patterns = [
    r"(?i)(?:current\s+version|version)\s*[:=]?\s*(v?\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.-]+)?)",
    r"(?<![A-Za-z0-9.])(v\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.-]+)?)",
]
for pattern in patterns:
    match = re.search(pattern, text)
    if match:
        print(match.group(1))
        break
else:
    print("unknown")
PY
}

if [[ ${1:-} == "--self-test-cleanup" && $# -eq 1 ]]; then
    python3 -c 'import signal, time; signal.signal(signal.SIGINT, signal.SIG_IGN); time.sleep(30)' &
    self_test_pid=$!
    sleep 0.1
    terminate_pid_bounded "$self_test_pid"
    if kill -0 "$self_test_pid" 2>/dev/null; then
        printf 'SELF_TEST_CHILD=REMAINS\n' >&2
        exit 1
    fi
    printf 'SELF_TEST_CHILD=GONE\n'
    printf 'CLEANUP_SELF_TEST=PASS\n'
    exit 0
fi

if [[ ${1:-} == "--self-test-version-parser" && $# -eq 1 ]]; then
    self_test_version=$(mktemp)
    trap 'rm -f "$self_test_version"' EXIT
    printf '\n   __        __\n[\033[34mINF\033[0m] Current version: v1.6.1\n' >"$self_test_version"
    parsed_version=$(extract_tool_version "$self_test_version")
    printf 'TOOL_VERSION=%s\n' "$parsed_version"
    [[ "$parsed_version" == "v1.6.1" ]] || exit 1
    printf 'VERSION_PARSER_SELF_TEST=PASS\n'
    exit 0
fi

if [[ ${1:-} == "--self-test-version-parser-arjun" && $# -eq 1 ]]; then
    self_test_help=$(mktemp)
    self_test_stdout=$(mktemp)
    trap 'rm -f "$self_test_help" "$self_test_stdout"' EXIT
    printf 'Default proxy is 127.0.0.1:8080.\n' >"$self_test_help"
    printf '\033[92m(  |/ /(//) v2.2.7\033[0m\n' >"$self_test_stdout"
    parsed_version=$(extract_tool_version "$self_test_help" "$self_test_stdout")
    printf 'TOOL_VERSION=%s\n' "$parsed_version"
    [[ "$parsed_version" == "v2.2.7" ]] || exit 1
    printf 'ARJUN_VERSION_PARSER_SELF_TEST=PASS\n'
    exit 0
fi

usage() {
    cat <<'EOF'
Usage:
  scripts/capture-failure-path.sh CASE [--preview|--execute]

Cases:
  katana-interrupt  Interrupt Katana after 3s on loopback
  arjun-interrupt   Interrupt Arjun after 6s on loopback
  arjun-timeout     Exercise Arjun request timeout against /slow

Default mode is --preview. Only the operator should use --execute.
EOF
}

[[ $# -ge 1 && $# -le 2 ]] || { usage >&2; exit 2; }
case_id=$1
mode=${2:---preview}
[[ "$mode" == "--preview" || "$mode" == "--execute" ]] || { usage >&2; exit 2; }

case "$case_id" in
    katana-interrupt)
        tool="katana"
        capture_id="KAT-INTERRUPTED-LOOPBACK"
        native_name="native-output.jsonl"
        expected="bounded outer interruption; partial JSONL may exist; no process may remain"
        ;;
    arjun-interrupt)
        tool="arjun"
        capture_id="ARJUN-INTERRUPTED-LOOPBACK"
        native_name="native-output.json"
        expected="bounded outer interruption; stdout/stderr preserved; native JSON may be absent or partial"
        ;;
    arjun-timeout)
        tool="arjun"
        capture_id="ARJUN-REQUEST-TIMEOUT-LOOPBACK"
        native_name="native-output.json"
        expected="request timeout against deterministic 3-5s response; process result and missing/partial native JSON preserved"
        ;;
    *)
        usage >&2
        exit 2
        ;;
esac

case_dir="private-captures/failure-paths/${capture_id}"
native_path="${case_dir}/${native_name}"
wordlist="fixtures/shared/arjun-minimal.txt"

case "$case_id" in
    katana-interrupt)
        command=(
            timeout --signal=INT --kill-after=2s 3s
            ./tools/bin/katana
            -u http://127.0.0.1:18080/
            -cs '^http://127\.0\.0\.1:18080(?:/|$)'
            -d 2 -j -nc -silent
            -rl 1 -c 1 -p 1 -timeout 10
            -or -ob
            -o "$native_path"
        )
        ;;
    arjun-interrupt)
        command=(
            timeout --signal=INT --kill-after=3s 6s
            ./tools/bin/arjun
            -u http://127.0.0.1:18080/api/search
            -m GET
            -w "$wordlist"
            -t 1 --rate-limit 1 -T 15
            -oJ "$native_path"
        )
        ;;
    arjun-timeout)
        command=(
            timeout --signal=TERM --kill-after=3s 30s
            ./tools/bin/arjun
            -u 'http://127.0.0.1:18080/slow?delay_ms=5000'
            -m GET
            -w "$wordlist"
            -t 1 --rate-limit 1 -T 1
            -oJ "$native_path"
        )
        ;;
esac

printf 'CASE=%s\n' "$capture_id"
printf 'TARGET=http://127.0.0.1:18080 (loopback only)\n'
printf 'EXPECTED=%s\n' "$expected"
printf 'COMMAND='
printf '%q ' "${command[@]}"
printf '> %q 2> %q\n' "${case_dir}/stdout.raw" "${case_dir}/stderr.raw"
printf 'OUTPUT=%s\n' "$case_dir"

if [[ "$mode" == "--preview" ]]; then
    printf 'MODE=PREVIEW_ONLY\n'
    exit 0
fi

printf 'MODE=OPERATOR_EXECUTE\n'
[[ -x "./tools/bin/${tool}" ]] || { printf 'ERROR: missing executable wrapper ./tools/bin/%s\n' "$tool" >&2; exit 1; }
[[ -f "$wordlist" ]] || { printf 'ERROR: missing wordlist %s\n' "$wordlist" >&2; exit 1; }
[[ ! -e "$case_dir" ]] || { printf 'ERROR: capture directory already exists; refusing append/overwrite: %s\n' "$case_dir" >&2; exit 1; }

if ss -ltn 'sport = :18080' | grep -q LISTEN; then
    printf 'ERROR: port 18080 already has a listener; refusing to reuse it\n' >&2
    exit 1
fi

mkdir -p "$case_dir"
target_tmp=$(mktemp -d)
target_pid=""
cleanup_running=false
cleanup() {
    [[ "$cleanup_running" == true ]] && return
    cleanup_running=true
    if [[ -n "$target_pid" ]]; then
        terminate_pid_bounded "$target_pid"
        target_pid=""
    fi
    rm -rf "$target_tmp"
    cleanup_running=false
}
on_signal() {
    local exit_code=$1
    trap - EXIT INT TERM
    cleanup
    exit "$exit_code"
}
trap cleanup EXIT
trap 'on_signal 130' INT
trap 'on_signal 143' TERM

python3 -m fixture_target.app --host 127.0.0.1 --port 18080 \
    >"$target_tmp/target.stdout" 2>"$target_tmp/target.stderr" &
target_pid=$!

ready=false
for _ in $(seq 1 40); do
    if curl --fail --silent --show-error http://127.0.0.1:18080/healthz >/dev/null 2>&1; then
        ready=true
        break
    fi
    sleep 0.1
done
[[ "$ready" == true ]] || { printf 'ERROR: fixture target did not become ready\n' >&2; exit 1; }

socket_state=$(ss -ltnp 'sport = :18080')
printf '%s\n' "$socket_state" >"$case_dir/socket-before.txt"
printf '%s\n' "$socket_state" | grep -q '127.0.0.1:18080' || { printf 'ERROR: fixture target is not bound to loopback\n' >&2; exit 1; }
if printf '%s\n' "$socket_state" | grep -qE '0\.0\.0\.0:18080|\[::\]:18080'; then
    printf 'ERROR: fixture target exposed beyond IPv4 loopback\n' >&2
    exit 1
fi

"./tools/bin/${tool}" --version >"$case_dir/version.txt" 2>&1 || \
    "./tools/bin/${tool}" -h >"$case_dir/version.txt" 2>&1 || true
printf '%q ' "${command[@]}" >"$case_dir/command.txt"
printf '> %q 2> %q\n' "${case_dir}/stdout.raw" "${case_dir}/stderr.raw" >>"$case_dir/command.txt"

started_at=$(date --iso-8601=seconds)
started_ms=$(date +%s%3N)
set +e
"${command[@]}" >"$case_dir/stdout.raw" 2>"$case_dir/stderr.raw"
exit_code=$?
set -e
finished_ms=$(date +%s%3N)
finished_at=$(date --iso-8601=seconds)
duration_ms=$((finished_ms - started_ms))
printf '%s\n' "$exit_code" >"$case_dir/exit-code.txt"

cleanup
target_pid=""
trap - EXIT INT TERM

if ss -ltn 'sport = :18080' | grep -q LISTEN; then
    printf 'ERROR: fixture target listener remains after cleanup\n' >&2
    exit 1
fi
printf 'PORT_18080_AFTER=CLOSED\n' >"$case_dir/socket-after.txt"
tool_version=$(extract_tool_version "$case_dir/version.txt" "$case_dir/stdout.raw")

python3 - "$case_dir" "$capture_id" "$tool" "$tool_version" "$started_at" "$finished_at" "$duration_ms" "$exit_code" "$case_id" "${command[@]}" <<'PY'
import json
import sys
from pathlib import Path

case_dir = Path(sys.argv[1])
capture_id, tool, version_text = sys.argv[2], sys.argv[3], sys.argv[4]
started_at, finished_at = sys.argv[5], sys.argv[6]
duration_ms, exit_code = int(sys.argv[7]), int(sys.argv[8])
case_id = sys.argv[9]
argv = sys.argv[10:]
native = [p.name for p in case_dir.glob("native-output.*") if p.is_file()]
outer_timeout = exit_code == 124
manifest = {
    "schema_version": "fixture-manifest/v0",
    "case_id": capture_id,
    "tool": tool,
    "tool_version": version_text,
    "origin": "captured",
    "source_case_id": None,
    "target_class": "loopback",
    "command_argv": argv,
    "command_display_redacted": (case_dir / "command.txt").read_text().strip(),
    "started_at": started_at,
    "finished_at": finished_at,
    "duration_ms": duration_ms,
    "exit_code": exit_code,
    "interrupted": case_id in {"katana-interrupt", "arjun-interrupt"} and outer_timeout,
    "timed_out": outer_timeout,
    "stdout_file": "stdout.raw",
    "stderr_file": "stderr.raw",
    "native_output_files": native,
    "checksums_file": "checksums.sha256",
    "environment_file": "environment.json",
    "sanitization": {
        "status": "pending",
        "ruleset_version": "sanitize/v0",
        "replacements": [],
    },
    "notes": [
        "operator-executed failure-path capture",
        "outer timeout exit 124 is recorded separately from tool semantic status",
    ],
}
(case_dir / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
environment = {
    "schema_version": "fixture-environment/v0",
    "platform": "linux",
    "target": "http://127.0.0.1:18080",
    "sensitive_environment_captured": False,
}
(case_dir / "environment.json").write_text(json.dumps(environment, indent=2, sort_keys=True) + "\n")
PY

(
    cd "$case_dir"
    files=(command.txt version.txt stdout.raw stderr.raw exit-code.txt environment.json manifest.json socket-before.txt socket-after.txt)
    [[ -f "$native_name" ]] && files+=("$native_name")
    sha256sum "${files[@]}" >checksums.sha256
    sha256sum -c checksums.sha256
)

printf 'CAPTURE_COMPLETE=%s\n' "$case_dir"
printf 'EXIT_CODE=%s\n' "$exit_code"
printf 'PORT_18080_AFTER=CLOSED\n'
