import http.client
import json
import threading
import time
import unittest
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from fixture_target.app import create_server


class FixtureTargetTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = create_server(host="127.0.0.1", port=0)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        host, port = cls.server.server_address
        cls.host = host
        cls.port = port
        cls.base_url = f"http://{host}:{port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def test_root_contains_expected_crawl_surface(self):
        with urlopen(f"{self.base_url}/", timeout=2) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "text/html")
            body = response.read().decode("utf-8")

        for expected in (
            '/login',
            '/search?q=seed',
            '/api/users?id=1',
            '/api/search',
            '/static/app.js',
            'http://outside.fixture.invalid/out-of-scope',
            'PROMPT_INJECTION_TEST_STRING',
        ):
            self.assertIn(expected, body)

        self.assertGreaterEqual(body.count('/search?q=seed'), 2)

    def test_login_exposes_expected_form_fields(self):
        with urlopen(f"{self.base_url}/login", timeout=2) as response:
            self.assertEqual(response.status, 200)
            body = response.read().decode("utf-8")

        self.assertIn('<form method="post" action="/login">', body)
        for field in ("user", "password", "next"):
            self.assertIn(f'name="{field}"', body)

    def get_json(self, path):
        with urlopen(f"{self.base_url}{path}", timeout=2) as response:
            self.assertEqual(response.status, 200)
            return json.load(response)

    def test_api_search_accepts_known_and_ignores_unknown_parameters(self):
        payload = self.get_json("/api/search?q=needle&debug=1&unknown_fixture_parameter=x")

        self.assertEqual(payload["endpoint"], "api-search")
        self.assertEqual(payload["accepted"], {"debug": "1", "q": "needle"})
        self.assertEqual(payload["ignored"], ["unknown_fixture_parameter"])

    def test_api_users_accepts_only_id(self):
        payload = self.get_json("/api/users?id=1&sort=asc")

        self.assertEqual(payload["endpoint"], "api-users")
        self.assertEqual(payload["accepted"], {"id": "1"})
        self.assertEqual(payload["ignored"], ["sort"])

    def test_api_update_accepts_known_form_fields(self):
        body = urlencode({"id": "7", "name": "fixture", "sort": "asc"}).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/update",
            data=body,
            headers={"Content-Type": "application/x-www-form-urlencoded"},
            method="POST",
        )
        with urlopen(request, timeout=2) as response:
            self.assertEqual(response.status, 200)
            payload = json.load(response)

        self.assertEqual(payload["endpoint"], "api-update")
        self.assertEqual(payload["accepted"], {"id": "7", "name": "fixture"})
        self.assertEqual(payload["ignored"], ["sort"])

    def test_api_json_accepts_known_json_fields(self):
        body = json.dumps(
            {"id": 9, "filter": "active", "debug": True, "sort": "asc"}
        ).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/json",
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urlopen(request, timeout=2) as response:
            self.assertEqual(response.status, 200)
            payload = json.load(response)

        self.assertEqual(payload["endpoint"], "api-json")
        self.assertEqual(
            payload["accepted"], {"debug": True, "filter": "active", "id": 9}
        )
        self.assertEqual(payload["ignored"], ["sort"])

    def test_api_no_params_ignores_everything(self):
        payload = self.get_json("/api/no-params?id=1&debug=1")

        self.assertEqual(payload["endpoint"], "api-no-params")
        self.assertEqual(payload["accepted"], {})
        self.assertEqual(payload["ignored"], ["debug", "id"])

    def test_search_page_renders_query_as_escaped_text(self):
        with urlopen(f"{self.base_url}/search?q=%3Cfixture%3E", timeout=2) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "text/html")
            body = response.read().decode("utf-8")

        self.assertIn("&lt;fixture&gt;", body)
        self.assertNotIn("<fixture>", body)

    def test_static_assets_have_expected_content_and_mime_types(self):
        with urlopen(f"{self.base_url}/static/app.js", timeout=2) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "application/javascript")
            javascript = response.read().decode("utf-8")

        for expected in ("/api/json", "/api/no-params", "outside.fixture.invalid"):
            self.assertIn(expected, javascript)

        with urlopen(f"{self.base_url}/static/app.css", timeout=2) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "text/css")
            self.assertIn("fixture-target", response.read().decode("utf-8"))

    def test_empty_page_has_no_links(self):
        with urlopen(f"{self.base_url}/empty", timeout=2) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "text/html")
            body = response.read().decode("utf-8")

        self.assertNotIn("href=", body)
        self.assertNotIn("src=", body)

    def test_redirect_stays_inside_fixture_target(self):
        connection = http.client.HTTPConnection(self.host, self.port, timeout=2)
        try:
            connection.request("GET", "/redirect")
            response = connection.getresponse()
            response.read()
        finally:
            connection.close()

        self.assertEqual(response.status, 302)
        self.assertEqual(response.getheader("Location"), "/search?q=redirected")

    def test_slow_route_applies_bounded_delay(self):
        started = time.monotonic()
        payload = self.get_json("/slow?delay_ms=50")
        elapsed = time.monotonic() - started

        self.assertEqual(payload, {"delay_ms": 50, "endpoint": "slow"})
        self.assertGreaterEqual(elapsed, 0.04)
        self.assertLess(elapsed, 0.5)

    def test_rate_limit_route_is_deterministic(self):
        statuses = []
        retry_after = None
        for _ in range(3):
            connection = http.client.HTTPConnection(self.host, self.port, timeout=2)
            try:
                connection.request("GET", "/rate-limit?key=unit-test&limit=2")
                response = connection.getresponse()
                statuses.append(response.status)
                retry_after = response.getheader("Retry-After") or retry_after
                response.read()
            finally:
                connection.close()

        self.assertEqual(statuses, [200, 200, 429])
        self.assertEqual(retry_after, "1")

    def test_server_rejects_non_loopback_bind(self):
        with self.assertRaisesRegex(ValueError, "loopback"):
            create_server(host="0.0.0.0", port=0)

    def test_healthz_returns_deterministic_json(self):
        with urlopen(f"{self.base_url}/healthz", timeout=2) as response:
            self.assertEqual(response.status, 200)
            self.assertEqual(response.headers.get_content_type(), "application/json")
            payload = json.load(response)

        self.assertEqual(payload, {"service": "fixture-target", "status": "ok"})


if __name__ == "__main__":
    unittest.main()
