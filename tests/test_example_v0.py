import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource

from reference.build_example_v0 import (
    build_agent_view,
    build_context,
    build_records,
    write_example,
)


ROOT = Path(__file__).parents[1]
SCHEMA_ROOT = ROOT / "schemas" / "v0"


class ExampleV0Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema_paths = sorted(SCHEMA_ROOT.glob("*.schema.json"))
        schemas = [json.loads(path.read_text()) for path in schema_paths]
        registry = Registry()
        for schema in schemas:
            registry = registry.with_resource(
                schema["$id"], Resource.from_contents(schema)
            )
        record_schema = next(
            item
            for item in schemas
            if item["$id"].endswith("/record.schema.json")
        )
        cls.validator = Draft202012Validator(
            record_schema, registry=registry, format_checker=FormatChecker()
        )

    def test_fixture_derived_records_validate_and_references_resolve(self):
        records, _ = build_records(ROOT)
        by_id = {record["id"]: record for record in records}

        self.assertEqual(len(by_id), len(records))
        for record in records:
            with self.subTest(record_id=record["id"]):
                self.validator.validate(record)

        for record in records:
            if "run_id" in record:
                self.assertIn(record["run_id"], by_id)
            for field in ("observation_ids", "evidence_ids"):
                for target in record.get(field, []):
                    self.assertIn(target, by_id)
            if record["record_type"] == "run":
                for target in record["tool_execution_ids"]:
                    self.assertIn(target, by_id)
            elif record["record_type"] == "endpoint":
                self.assertIn(record["origin_asset_id"], by_id)
            elif record["record_type"] == "parameter":
                self.assertIn(record["endpoint_id"], by_id)
            elif record["record_type"] == "observation":
                self.assertIn(record["tool_execution_id"], by_id)
                self.assertIn(record["subject"]["id"], by_id)
            elif record["record_type"] == "evidence":
                self.assertIn(record["tool_execution_id"], by_id)
            elif record["record_type"] == "relationship":
                self.assertIn(record["from_ref"]["id"], by_id)
                self.assertIn(record["to_ref"]["id"], by_id)

    def test_fixture_semantics_survive_normalization(self):
        records, _ = build_records(ROOT)
        endpoints = [x for x in records if x["record_type"] == "endpoint"]
        observations = [x for x in records if x["record_type"] == "observation"]
        executions = [x for x in records if x["record_type"] == "tool_execution"]

        api_search = next(
            x
            for x in endpoints
            if x["canonical_route_url"]
            == "http://127.0.0.1:18080/api/search"
            and x["method"] == "GET"
        )
        api_search_types = {
            x["observation_type"]
            for x in observations
            if x["id"] in api_search["observation_ids"]
        }
        self.assertEqual(api_search_types, {"http_response", "parameter_discovery"})

        gau_endpoints = [
            x for x in endpoints if x["host"].endswith("fixture.test")
        ]
        self.assertEqual(len(gau_endpoints), 2)
        self.assertTrue(all(x["method"] is None for x in gau_endpoints))
        historical = [
            x for x in observations if x["observation_type"] == "historical_url"
        ]
        self.assertEqual(len(historical), 3)

        zero = next(x for x in executions if x["id"] == "tx_fixture_arjun_zero")
        self.assertEqual(zero["status"], "success_zero")
        native = next(x for x in zero["artifacts"] if x["role"] == "native_output")
        self.assertFalse(native["present"])

    def test_compact_context_is_complete_resolvable_and_smaller_than_raw(self):
        records, raw_files = build_records(ROOT)
        context = build_context(records)
        endpoints = [x for x in records if x["record_type"] == "endpoint"]
        evidence = [x for x in records if x["record_type"] == "evidence"]

        self.assertLess(len(context.encode()), sum(len(x) for x in raw_files.values()))
        self.assertIn("## Surface", context)
        self.assertIn("## Arjun parameters", context)
        self.assertIn("## Multi-source", context)
        self.assertIn("## Gaps and prohibited claims", context)
        self.assertIn("No vulnerability is confirmed", context)
        for endpoint in endpoints:
            self.assertIn(endpoint["canonical_route_url"], context)
        for item in evidence:
            self.assertIn(item["id"], context)
            self.assertIn(item["artifact"]["path"], context)

    def test_agent_view_has_one_deterministic_resolvable_row_per_endpoint(self):
        records, _ = build_records(ROOT)
        first = build_agent_view(records)
        second = build_agent_view(list(reversed(records)))
        endpoints = {x["id"] for x in records if x["record_type"] == "endpoint"}
        evidence = {x["id"] for x in records if x["record_type"] == "evidence"}

        self.assertEqual(first, second)
        self.assertEqual(len(first), 11)
        self.assertEqual({x["endpoint_id"] for x in first}, endpoints)
        self.assertTrue(all(x["view_version"] == "reconctx-agent-view/v0" for x in first))
        for row in first:
            self.assertTrue(set(row["evidence_ids"]).issubset(evidence))
            self.assertEqual(row["evidence_ids"], sorted(set(row["evidence_ids"])))
            self.assertEqual(row["sources"], sorted(set(row["sources"])))

    def test_write_example_creates_portable_jsonl_and_raw_artifacts(self):
        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory)
            write_example(ROOT, destination)

            records = [
                json.loads(line)
                for line in (destination / "normalized" / "records.jsonl")
                .read_text()
                .splitlines()
                if line.strip()
            ]
            self.assertGreater(len(records), 0)
            self.assertTrue((destination / "raw" / "gau" / "native-output.txt").exists())
            self.assertTrue((destination / "raw" / "katana" / "native-output.jsonl").exists())
            self.assertTrue((destination / "raw" / "arjun-zero" / "stdout.raw").exists())
            agent_view = [
                json.loads(line)
                for line in (destination / "normalized" / "agent-view.jsonl")
                .read_text()
                .splitlines()
                if line.strip()
            ]
            self.assertEqual(len(agent_view), 11)
            self.assertLess(
                (destination / "CONTEXT.md").stat().st_size,
                sum(path.stat().st_size for path in (destination / "raw").rglob("*") if path.is_file()),
            )

            manifest = json.loads((destination / "manifest.json").read_text())
            for packaged in manifest["files"]:
                content = (destination / packaged["path"]).read_bytes()
                self.assertEqual(len(content), packaged["size_bytes"])
                self.assertEqual(
                    hashlib.sha256(content).hexdigest(), packaged["sha256"]
                )


if __name__ == "__main__":
    unittest.main()
