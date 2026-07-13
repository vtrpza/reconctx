#!/usr/bin/env python3
import argparse
import signal
import subprocess
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument("--mode", choices=("complete", "hang"), required=True)
args = parser.parse_args()

print("started", flush=True)
print("progress", file=sys.stderr, flush=True)

if args.mode == "complete":
    print('{"result":"ok"}', flush=True)
    raise SystemExit(0)

signal.signal(signal.SIGTERM, signal.SIG_IGN)
subprocess.Popen([sys.executable, "-c", "import time; time.sleep(30)"])
print("child_started", file=sys.stderr, flush=True)
while True:
    time.sleep(1)
