import argparse
import statistics
import tempfile
import time
from pathlib import Path

from spike import compile_fixture

parser = argparse.ArgumentParser()
parser.add_argument("fixture", type=Path)
parser.add_argument("--iterations", type=int, default=500)
args = parser.parse_args()

durations = []
with tempfile.TemporaryDirectory() as root:
    for index in range(args.iterations):
        output = Path(root) / str(index)
        started = time.perf_counter_ns()
        compile_fixture(args.fixture, output)
        durations.append(time.perf_counter_ns() - started)

print(f"iterations={args.iterations}")
print(f"median_ns_per_op={int(statistics.median(durations))}")
print(f"mean_ns_per_op={int(statistics.mean(durations))}")
