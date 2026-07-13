import subprocess
from pathlib import Path
import unittest


ROOT = Path(__file__).parents[1]
SCRIPT = ROOT / "scripts" / "capture-failure-path.sh"


class CaptureFailurePathHarnessTests(unittest.TestCase):
    def test_cleanup_self_test_kills_child_that_ignores_sigint(self):
        completed = subprocess.run(
            ["bash", str(SCRIPT), "--self-test-cleanup"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=8,
            check=False,
        )

        self.assertEqual(completed.returncode, 0, completed.stderr + completed.stdout)
        self.assertIn("CLEANUP_SELF_TEST=PASS", completed.stdout)
        self.assertIn("SELF_TEST_CHILD=GONE", completed.stdout)

    def test_version_parser_skips_banner_and_ansi_lines(self):
        completed = subprocess.run(
            ["bash", str(SCRIPT), "--self-test-version-parser"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )

        self.assertEqual(completed.returncode, 0, completed.stderr + completed.stdout)
        self.assertIn("TOOL_VERSION=v1.6.1", completed.stdout)
        self.assertIn("VERSION_PARSER_SELF_TEST=PASS", completed.stdout)

    def test_version_parser_ignores_ip_and_uses_arjun_runtime_banner(self):
        completed = subprocess.run(
            ["bash", str(SCRIPT), "--self-test-version-parser-arjun"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )

        self.assertEqual(completed.returncode, 0, completed.stderr + completed.stdout)
        self.assertIn("TOOL_VERSION=v2.2.7", completed.stdout)
        self.assertIn("ARJUN_VERSION_PARSER_SELF_TEST=PASS", completed.stdout)


if __name__ == "__main__":
    unittest.main()
