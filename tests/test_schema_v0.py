import json
from pathlib import Path
import unittest

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource


SCHEMA_ROOT = Path(__file__).parents[1] / "schemas" / "v0"
SCHEMA_FILES = [
    "common.schema.json",
    "run.schema.json",
    "tool-execution.schema.json",
    "asset.schema.json",
    "endpoint.schema.json",
    "parameter.schema.json",
    "observation.schema.json",
    "evidence.schema.json",
    "relationship.schema.json",
    "record.schema.json",
    "handoff-manifest.schema.json",
]
HEX0 = "0" * 64
HEX1 = "1" * 64
HEX2 = "2" * 64


class SchemaV0Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schemas = {
            name: json.loads((SCHEMA_ROOT / name).read_text())
            for name in SCHEMA_FILES
        }
        registry = Registry()
        for schema in cls.schemas.values():
            registry = registry.with_resource(
                schema["$id"], Resource.from_contents(schema)
            )
        cls.validator = Draft202012Validator(
            cls.schemas["record.schema.json"],
            registry=registry,
            format_checker=FormatChecker(),
        )

    def test_all_schema_documents_are_valid_draft_2020_12(self):
        for name, schema in self.schemas.items():
            with self.subTest(schema=name):
                Draft202012Validator.check_schema(schema)

    def test_one_valid_record_of_every_type(self):
        records = self._valid_records()
        self.assertEqual(
            {record["record_type"] for record in records},
            {
                "run",
                "tool_execution",
                "asset",
                "endpoint",
                "parameter",
                "observation",
                "evidence",
                "relationship",
            },
        )
        for record in records:
            with self.subTest(record_type=record["record_type"]):
                self.validator.validate(record)

    def test_unknown_method_endpoint_requires_null_method(self):
        endpoint = next(
            item for item in self._valid_records() if item["record_type"] == "endpoint"
        )
        endpoint["method_known"] = False
        endpoint["method"] = "GET"

        errors = list(self.validator.iter_errors(endpoint))
        self.assertTrue(errors)

    def test_success_zero_tool_execution_allows_absent_native_output(self):
        execution = next(
            item
            for item in self._valid_records()
            if item["record_type"] == "tool_execution"
        )

        self.assertEqual(execution["status"], "success_zero")
        self.assertEqual(execution["coverage"], "zero")
        self.assertFalse(execution["artifacts"][0]["present"])
        self.validator.validate(execution)

    def test_unknown_top_level_fields_are_rejected(self):
        asset = next(
            item for item in self._valid_records() if item["record_type"] == "asset"
        )
        asset["invented"] = True

        errors = list(self.validator.iter_errors(asset))
        self.assertTrue(errors)

    def test_auth_context_is_opaque_and_never_a_raw_secret(self):
        observation = next(
            item
            for item in self._valid_records()
            if item["record_type"] == "observation"
        )
        observation["auth_context_id"] = "authctx_user_a"
        self.validator.validate(observation)

        observation["auth_context_id"] = "not-an-auth-context"
        self.assertTrue(list(self.validator.iter_errors(observation)))

    def test_materialized_handoff_manifest_validates(self):
        manifest = json.loads(
            (
                Path(__file__).parents[1]
                / "examples"
                / "handoff-web-blackbox-v0"
                / "manifest.json"
            ).read_text()
        )
        Draft202012Validator(
            self.schemas["handoff-manifest.schema.json"],
            format_checker=FormatChecker(),
        ).validate(manifest)

    @staticmethod
    def _valid_records():
        run_id = "run_fixture_v0"
        tx_id = "tx_fixture_arjun_zero"
        asset_id = f"asset_sha256_{HEX0}"
        endpoint_id = f"ep_sha256_{HEX0}"
        parameter_id = f"param_sha256_{HEX0}"
        evidence_id = f"ev_sha256_{HEX1}"
        observation_id = f"obs_sha256_{HEX1}"
        relationship_id = f"rel_sha256_{HEX2}"
        scope = {
            "classification": "in_scope",
            "rule_id": "scope_loopback",
            "reason": "origin allowlisted",
        }
        return [
            {
                "schema_version": "reconctx/v0",
                "record_type": "run",
                "id": run_id,
                "created_at": "2026-07-12T22:00:00-03:00",
                "status": "success",
                "canonicalization_policy": "url-canonicalization/v0",
                "scope": {
                    "mode": "allowlist",
                    "roots": [
                        {"kind": "origin", "value": "http://127.0.0.1:18080"}
                    ],
                    "external_policy": "reject",
                    "approved_by": "operator",
                    "approved_at": "2026-07-12T21:57:00-03:00",
                },
                "tool_execution_ids": [tx_id],
                "warnings": [],
                "gaps": [],
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "tool_execution",
                "id": tx_id,
                "run_id": run_id,
                "tool": {
                    "name": "arjun",
                    "version": "2.2.7",
                    "resolved_path": "tools/bin/arjun",
                },
                "adapter_version": "arjun-adapter/v0",
                "auth_context_id": None,
                "activity_class": "active_local",
                "approval_phase": "parameter_discovery",
                "argv_redacted": [
                    "tools/bin/arjun",
                    "-u",
                    "http://127.0.0.1:18080/api/no-params",
                ],
                "started_at": "2026-07-12T22:00:29.011819-03:00",
                "finished_at": "2026-07-12T22:00:35.189730-03:00",
                "duration_ms": 6178,
                "exit_code": 0,
                "status": "success_zero",
                "coverage": "zero",
                "artifacts": [
                    {
                        "role": "native_output",
                        "path": "raw/arjun-zero/native-output.json",
                        "present": False,
                        "sha256": None,
                        "size_bytes": None,
                        "media_type": "application/json",
                    },
                    {
                        "role": "stdout",
                        "path": "raw/arjun-zero/stdout.raw",
                        "present": True,
                        "sha256": HEX0,
                        "size_bytes": 558,
                        "media_type": "text/plain",
                    },
                ],
                "provider_status": [],
                "warnings": [],
                "gaps": [],
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "asset",
                "id": asset_id,
                "run_id": run_id,
                "asset_kind": "origin",
                "canonical_value": "http://127.0.0.1:18080",
                "display_value": "http://127.0.0.1:18080",
                "scope_decision": scope,
                "observation_ids": [observation_id],
                "evidence_ids": [evidence_id],
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "endpoint",
                "id": endpoint_id,
                "run_id": run_id,
                "origin_asset_id": asset_id,
                "canonical_route_url": "https://finance.fixture.test/",
                "scheme": "https",
                "host": "finance.fixture.test",
                "port": None,
                "path": "/",
                "method": None,
                "method_known": False,
                "route_template": None,
                "scope_decision": scope,
                "observation_ids": [observation_id],
                "evidence_ids": [evidence_id],
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "parameter",
                "id": parameter_id,
                "run_id": run_id,
                "endpoint_id": endpoint_id,
                "name": "q",
                "location": "query",
                "discovery_kinds": ["bruteforced"],
                "observation_ids": [observation_id],
                "evidence_ids": [evidence_id],
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "observation",
                "id": observation_id,
                "run_id": run_id,
                "tool_execution_id": tx_id,
                "auth_context_id": None,
                "observation_type": "zero_result",
                "semantic_state": "observed",
                "subject": {"record_type": "tool_execution", "id": tx_id},
                "scope_decision": scope,
                "observed_at": "2026-07-12T22:00:35.189730-03:00",
                "evidence_ids": [evidence_id],
                "details": {
                    "result_kind": "parameter_discovery",
                    "target_url": "http://127.0.0.1:18080/api/no-params",
                    "message": "No parameters were discovered.",
                },
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "evidence",
                "id": evidence_id,
                "run_id": run_id,
                "tool_execution_id": tx_id,
                "artifact": {
                    "role": "stdout",
                    "path": "raw/arjun-zero/stdout.raw",
                    "sha256": HEX0,
                    "size_bytes": 558,
                    "media_type": "text/plain",
                    "sanitized": True,
                },
                "locator": {"kind": "line_range", "line_start": 11, "line_end": 11},
                "redaction_status": "not_needed",
                "scope_decision": scope,
            },
            {
                "schema_version": "reconctx/v0",
                "record_type": "relationship",
                "id": relationship_id,
                "run_id": run_id,
                "relationship_type": "evidence_for",
                "from_ref": {"record_type": "evidence", "id": evidence_id},
                "to_ref": {"record_type": "observation", "id": observation_id},
                "evidence_ids": [evidence_id],
                "attributes": {},
            },
        ]


if __name__ == "__main__":
    unittest.main()
