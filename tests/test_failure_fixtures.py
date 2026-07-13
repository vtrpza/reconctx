import hashlib
import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).parents[1]
FIXTURES = ROOT / "fixtures" / "cases"

CASES = {
    "KAT-INTERRUPTED-LOOPBACK": FIXTURES
    / "katana"
    / "v1.6.1"
    / "KAT-INTERRUPTED-LOOPBACK",
    "ARJUN-INTERRUPTED-LOOPBACK": FIXTURES
    / "arjun"
    / "2.2.7"
    / "ARJUN-INTERRUPTED-LOOPBACK",
    "ARJUN-REQUEST-TIMEOUT-LOOPBACK": FIXTURES
    / "arjun"
    / "2.2.7"
    / "ARJUN-REQUEST-TIMEOUT-LOOPBACK",
}


class FailureFixtureTests(unittest.TestCase):
    def load(self, case_id, name):
        return json.loads((CASES[case_id] / name).read_text())

    def test_public_failure_fixtures_exist_and_checksums_resolve(self):
        for case_id, case_dir in CASES.items():
            with self.subTest(case_id=case_id):
                self.assertTrue(case_dir.is_dir())
                lines = (case_dir / "checksums.sha256").read_text().splitlines()
                self.assertGreater(len(lines), 0)
                for line in lines:
                    digest, name = line.split("  ", 1)
                    content = (case_dir / name).read_bytes()
                    self.assertEqual(hashlib.sha256(content).hexdigest(), digest)

    def test_failure_semantics_are_explicit(self):
        katana = self.load("KAT-INTERRUPTED-LOOPBACK", "manifest.json")
        katana_expected = self.load("KAT-INTERRUPTED-LOOPBACK", "expected.json")
        arjun_interrupt = self.load("ARJUN-INTERRUPTED-LOOPBACK", "manifest.json")
        arjun_interrupt_expected = self.load(
            "ARJUN-INTERRUPTED-LOOPBACK", "expected.json"
        )
        arjun_timeout = self.load("ARJUN-REQUEST-TIMEOUT-LOOPBACK", "manifest.json")
        arjun_timeout_expected = self.load(
            "ARJUN-REQUEST-TIMEOUT-LOOPBACK", "expected.json"
        )

        self.assertEqual(katana["tool_version"], "v1.6.1")
        self.assertEqual(katana["exit_code"], 124)
        self.assertTrue(katana["interrupted"])
        self.assertEqual(katana_expected["parse_status"], "partial")
        lines = [
            json.loads(line)
            for line in (CASES["KAT-INTERRUPTED-LOOPBACK"] / "native-output.jsonl")
            .read_text()
            .splitlines()
            if line.strip()
        ]
        self.assertEqual(len(lines), 3)

        self.assertEqual(arjun_interrupt["tool_version"], "2.2.7")
        self.assertEqual(arjun_interrupt["exit_code"], 124)
        self.assertTrue(arjun_interrupt["interrupted"])
        self.assertEqual(arjun_interrupt["native_output_files"], [])
        self.assertEqual(arjun_interrupt_expected["parse_status"], "partial")
        self.assertEqual(arjun_interrupt_expected["semantic_status"], "interrupted")

        self.assertEqual(arjun_timeout["tool_version"], "2.2.7")
        self.assertEqual(arjun_timeout["exit_code"], 1)
        self.assertFalse(arjun_timeout["timed_out"])
        self.assertEqual(arjun_timeout["native_output_files"], [])
        self.assertEqual(arjun_timeout_expected["parse_status"], "tool_error")
        self.assertEqual(arjun_timeout_expected["semantic_status"], "failed")
        self.assertIn(
            "AttributeError: 'str' object has no attribute 'status_code'",
            (CASES["ARJUN-REQUEST-TIMEOUT-LOOPBACK"] / "stderr.sanitized.log")
            .read_text(),
        )

    def test_public_failure_fixtures_are_sanitized(self):
        prohibited = [
            re.compile(r"/home/[^/\s]+"),
            re.compile(r"Authorization\s*:"),
            re.compile(r"Cookie\s*:"),
            re.compile(r"Bearer\s+"),
            re.compile(r"BEGIN [A-Z ]*PRIVATE KEY"),
        ]
        for case_id, case_dir in CASES.items():
            with self.subTest(case_id=case_id):
                manifest = self.load(case_id, "manifest.json")
                self.assertEqual(manifest["sanitization"]["status"], "passed")
                for path in case_dir.iterdir():
                    if path.is_file():
                        text = path.read_text(errors="ignore")
                        self.assertFalse(
                            any(pattern.search(text) for pattern in prohibited),
                            path,
                        )


if __name__ == "__main__":
    unittest.main()
