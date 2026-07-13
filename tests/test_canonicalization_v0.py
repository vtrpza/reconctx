import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import idna

from reference.canonicalization_v0 import (
    CanonicalizationError,
    canonicalize_url,
    endpoint_id,
    normalize_source_method,
    parameter_id,
)


class CanonicalizationV0Tests(unittest.TestCase):
    def test_go_and_python_canonicalizers_match_directly(self):
        self.maxDiff = None
        self.assertEqual(idna.__version__, "3.4")
        repository = Path(__file__).parents[1]
        inputs = [
            f"https://a{chr(codepoint)}b.example/p?x=1#f"
            for codepoint in range(0x80, 0xA000)
        ]
        inputs.extend(
            f"https://{host}/p?x=1#f"
            for host in (
                "l·l.example",
                "͵α.example",
                "א׳.example",
                "カ・ナ.example",
                "١٢.example",
                "۱۲.example",
                "١۲.example",
            )
        )
        payload = "".join(json.dumps(value) + "\n" for value in inputs)

        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / "canonical.test"
            subprocess.run(
                ["go", "test", "-c", "-o", str(binary), "./internal/canonical"],
                cwd=repository,
                check=True,
                capture_output=True,
                text=True,
            )
            environment = os.environ.copy()
            environment["RECONCTX_DIFFERENTIAL_HELPER"] = "1"
            process = subprocess.run(
                [str(binary), "-test.run", "^TestURLDifferentialHelper$"],
                input=payload,
                capture_output=True,
                text=True,
                env=environment,
                timeout=120,
            )
        self.assertEqual(process.returncode, 0, process.stderr or process.stdout)
        responses = [
            json.loads(line)
            for line in process.stdout.splitlines()
            if line.startswith("{")
        ]
        self.assertEqual(len(responses), len(inputs), process.stdout[-2000:])

        differences = []
        for raw_url, response in zip(inputs, responses, strict=True):
            try:
                expected = canonicalize_url(raw_url)
                expected_error = False
            except CanonicalizationError:
                expected = None
                expected_error = True
            if response["error"] != expected_error or (
                not expected_error and response.get("value") != expected
            ):
                differences.append(
                    {
                        "input": raw_url,
                        "python_error": expected_error,
                        "go_error": response["error"],
                        "python_host": expected and expected["host"],
                        "go_host": response.get("value", {}).get("host"),
                    }
                )
                if len(differences) == 20:
                    break
        self.assertEqual(differences, [])

    def test_machine_readable_vectors_match_reference(self):
        vector_path = (
            Path(__file__).parents[1]
            / "fixtures"
            / "canonicalization"
            / "v0"
            / "vectors.json"
        )
        vectors = json.loads(vector_path.read_text())

        for case in vectors["url_cases"]:
            with self.subTest(case=case["id"]):
                if "error" in case:
                    with self.assertRaises(CanonicalizationError):
                        canonicalize_url(case["input"])
                    continue
                actual = canonicalize_url(case["input"])
                self.assertEqual(actual, case["expected"])

    def test_lowercases_origin_removes_default_port_and_fragment(self):
        result = canonicalize_url(
            "HTTP://Example.COM:80?b=2&a=1&a&empty=#section"
        )

        self.assertEqual(result["origin"], "http://example.com")
        self.assertEqual(result["path"], "/")
        self.assertEqual(result["canonical_route_url"], "http://example.com/")
        self.assertEqual(
            result["canonical_observation_url"],
            "http://example.com/?b=2&a=1&a&empty=",
        )
        self.assertTrue(result["fragment_present"])
        self.assertEqual(result["fragment_raw"], "section")
        self.assertEqual(
            [(p["name"], p["value"], p["has_equals"]) for p in result["query_pairs"]],
            [("b", "2", True), ("a", "1", True), ("a", None, False), ("empty", "", True)],
        )

    def test_normalizes_dot_segments_and_percent_encoding(self):
        result = canonicalize_url(
            "https://EXAMPLE.com/a/./b/../%7euser/%2f?q=%7e+x%2f"
        )

        self.assertEqual(result["path"], "/a/~user/%2F")
        self.assertEqual(
            result["canonical_observation_url"],
            "https://example.com/a/~user/%2F?q=~+x%2F",
        )

    def test_preserves_path_case_repeated_slashes_and_trailing_slash(self):
        result = canonicalize_url("https://example.com/A//b/")
        self.assertEqual(result["path"], "/A//b/")

    def test_query_order_and_repetitions_do_not_change_endpoint_identity(self):
        first = canonicalize_url("https://example.com/users?id=1&id=2&sort=asc")
        second = canonicalize_url("https://example.com/users?sort=asc&id=2&id=1")

        self.assertEqual(first["canonical_route_url"], second["canonical_route_url"])
        self.assertNotEqual(
            first["canonical_observation_url"], second["canonical_observation_url"]
        )
        self.assertEqual(
            endpoint_id("GET", first["canonical_route_url"]),
            endpoint_id("get", second["canonical_route_url"]),
        )

    def test_known_and_unknown_methods_produce_different_endpoint_ids(self):
        route = "https://example.com/users"
        known = endpoint_id("GET", route)
        unknown = endpoint_id(None, route)
        literal_star = endpoint_id("*", route)

        self.assertRegex(known, r"^ep_sha256_[0-9a-f]{64}$")
        self.assertNotEqual(known, unknown)
        self.assertNotEqual(literal_star, unknown)

    def test_parameter_identity_is_case_and_location_sensitive(self):
        endpoint = endpoint_id("POST", "https://example.com/users")

        self.assertNotEqual(
            parameter_id(endpoint, "form", "id"),
            parameter_id(endpoint, "json", "id"),
        )
        self.assertNotEqual(
            parameter_id(endpoint, "json", "id"),
            parameter_id(endpoint, "json", "ID"),
        )

    def test_arjun_json_is_source_mode_not_http_method(self):
        result = normalize_source_method("JSON", tool="arjun")

        self.assertEqual(
            result,
            {
                "source_label": "JSON",
                "http_method": "POST",
                "method_known": True,
                "body_kind": "json",
                "parameter_location": "json",
            },
        )

    def test_gau_method_remains_unknown(self):
        result = normalize_source_method(None, tool="gau")
        self.assertIsNone(result["http_method"])
        self.assertFalse(result["method_known"])

    def test_unicode_host_and_path_are_canonicalized(self):
        result = canonicalize_url("https://BÜCHER.example/é")

        self.assertEqual(result["host"], "xn--bcher-kva.example")
        self.assertEqual(result["path"], "/%C3%A9")

    def test_ipv6_is_compressed_and_default_port_removed(self):
        result = canonicalize_url("http://[2001:0db8::1]:80/a")

        self.assertEqual(result["host"], "2001:db8::1")
        self.assertEqual(result["origin"], "http://[2001:db8::1]")

    def test_rejects_unsupported_or_ambiguous_urls(self):
        invalid = [
            "/relative",
            "ftp://example.com/file",
            "https://user:pass@example.com/",
            "https://example.com/%ZZ",
            "https://example.com:99999/",
        ]

        for value in invalid:
            with self.subTest(value=value):
                with self.assertRaises(CanonicalizationError):
                    canonicalize_url(value)


if __name__ == "__main__":
    unittest.main()
