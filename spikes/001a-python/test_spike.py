import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE))

from spike import compile_fixture, run_supervised


class PythonSpikeTests(unittest.TestCase):
    def test_compiles_katana_jsonl_into_events_and_context(self):
        fixture = ROOT / "fixtures/cases/katana/v1.6.1/KAT-NORMAL-MINIMAL/native-output.jsonl"
        with tempfile.TemporaryDirectory() as tmp:
            summary = compile_fixture(fixture, Path(tmp))
            self.assertEqual(summary["records"], 6)
            events = [json.loads(line) for line in (Path(tmp) / "events.jsonl").read_text().splitlines()]
            self.assertEqual(len(events), 6)
            users = next(event for event in events if "api/users" in event["url_raw"])
            self.assertEqual(users["url_raw"], "http://127.0.0.1:18080/api/users?id=1")
            self.assertEqual(users["route_url"], "http://127.0.0.1:18080/api/users")
            self.assertEqual(users["method"], "GET")
            self.assertEqual(users["status_code"], 200)
            context = (Path(tmp) / "CONTEXT.md").read_text()
            self.assertIn("Records: 6", context)
            self.assertIn("Unique routes: 6", context)

    def test_cli_compile_materializes_artifacts_and_prints_summary(self):
        fixture = ROOT / "fixtures/cases/katana/v1.6.1/KAT-NORMAL-MINIMAL/native-output.jsonl"
        with tempfile.TemporaryDirectory() as tmp:
            result = subprocess.run(
                [sys.executable, str(HERE / "spike.py"), "compile", str(fixture), tmp],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout), {"records": 6, "unique_routes": 6})
            self.assertTrue((Path(tmp) / "events.jsonl").is_file())
            self.assertTrue((Path(tmp) / "CONTEXT.md").is_file())

    def test_timeout_kills_fake_process_group_and_preserves_streams(self):
        result = run_supervised(
            [sys.executable, str(ROOT / "spikes/fake_tool.py"), "--mode", "hang"],
            timeout_seconds=0.25,
            grace_seconds=0.1,
        )
        self.assertTrue(result["timed_out"])
        self.assertLess(result["duration_seconds"], 2.0)
        self.assertIn("started", result["stdout"])
        self.assertIn("child_started", result["stderr"])
        self.assertNotEqual(result["exit_code"], 0)


if __name__ == "__main__":
    unittest.main()
