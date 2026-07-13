import argparse
import json
import os
import signal
import subprocess
import time
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit


def _route_url(raw_url: str) -> str:
    parsed = urlsplit(raw_url)
    return urlunsplit((parsed.scheme.lower(), parsed.netloc.lower(), parsed.path or "/", "", ""))


def compile_fixture(source: Path, output_dir: Path) -> dict:
    source = Path(source)
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    events = []
    with source.open("r", encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, 1):
            if not line.strip():
                continue
            native = json.loads(line)
            raw_url = native["request"]["endpoint"]
            events.append(
                {
                    "source_line": line_number,
                    "timestamp": native.get("timestamp"),
                    "method": native["request"].get("method"),
                    "url_raw": raw_url,
                    "route_url": _route_url(raw_url),
                    "status_code": native.get("response", {}).get("status_code"),
                }
            )
    with (output_dir / "events.jsonl").open("w", encoding="utf-8", newline="\n") as handle:
        for event in events:
            handle.write(json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n")
    routes = {event["route_url"] for event in events}
    (output_dir / "CONTEXT.md").write_text(
        "# Stack Spike Context\n\n"
        f"- Records: {len(events)}\n"
        f"- Unique routes: {len(routes)}\n",
        encoding="utf-8",
    )
    return {"records": len(events), "unique_routes": len(routes)}


def run_supervised(argv: list[str], timeout_seconds: float, grace_seconds: float) -> dict:
    started = time.monotonic()
    process = subprocess.Popen(
        argv,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        start_new_session=True,
    )
    timed_out = False
    try:
        stdout, stderr = process.communicate(timeout=timeout_seconds)
    except subprocess.TimeoutExpired:
        timed_out = True
        os.killpg(process.pid, signal.SIGTERM)
        try:
            stdout, stderr = process.communicate(timeout=grace_seconds)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            stdout, stderr = process.communicate()
    return {
        "exit_code": process.returncode,
        "timed_out": timed_out,
        "duration_seconds": time.monotonic() - started,
        "stdout": stdout,
        "stderr": stderr,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    subcommands = parser.add_subparsers(dest="command", required=True)
    compile_command = subcommands.add_parser("compile")
    compile_command.add_argument("source", type=Path)
    compile_command.add_argument("output_dir", type=Path)
    args = parser.parse_args()
    summary = compile_fixture(args.source, args.output_dir)
    print(json.dumps(summary, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
