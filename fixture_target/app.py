import argparse
import json
import threading
import time
from html import escape
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit


class FixtureRequestHandler(BaseHTTPRequestHandler):
    server_version = "FixtureTarget/0.1"

    def do_GET(self):
        parsed = urlsplit(self.path)
        path = parsed.path
        query = parse_qs(parsed.query, keep_blank_values=True)

        if path == "/healthz":
            self._send_json(200, {"service": "fixture-target", "status": "ok"})
            return
        if path == "/api/no-params":
            self._send_json(
                200,
                {
                    "accepted": {},
                    "endpoint": "api-no-params",
                    "ignored": sorted(query),
                },
            )
            return
        if path == "/api/users":
            known = {key: values[-1] for key, values in query.items() if key == "id"}
            ignored = sorted(key for key in query if key != "id")
            self._send_json(
                200,
                {"accepted": known, "endpoint": "api-users", "ignored": ignored},
            )
            return
        if path == "/api/search":
            known = {key: values[-1] for key, values in query.items() if key in {"q", "debug"}}
            ignored = sorted(key for key in query if key not in {"q", "debug"})
            self._send_json(
                200,
                {"accepted": known, "endpoint": "api-search", "ignored": ignored},
            )
            return
        if path == "/static/app.js":
            self._send_text(
                200,
                "application/javascript; charset=utf-8",
                """const endpoints = ['/api/json', '/api/no-params', '/api/update'];
const externalFixture = 'http://outside.fixture.invalid/from-javascript';
""",
            )
            return
        if path == "/static/app.css":
            self._send_text(
                200,
                "text/css; charset=utf-8",
                "body.fixture-target { color: #123456; }\n",
            )
            return
        if path == "/rate-limit":
            key = query.get("key", ["default"])[-1]
            try:
                requested_limit = int(query.get("limit", ["2"])[-1])
            except ValueError:
                requested_limit = 2
            limit = max(1, min(requested_limit, 100))
            with self.server.rate_limit_lock:
                count = self.server.rate_limit_counts.get(key, 0) + 1
                self.server.rate_limit_counts[key] = count
            if count > limit:
                self._send_json(
                    429,
                    {"count": count, "endpoint": "rate-limit", "limit": limit},
                    headers={"Retry-After": "1"},
                )
            else:
                self._send_json(
                    200,
                    {"count": count, "endpoint": "rate-limit", "limit": limit},
                )
            return
        if path == "/slow":
            try:
                requested_delay = int(query.get("delay_ms", ["3000"])[-1])
            except ValueError:
                requested_delay = 3000
            delay_ms = max(0, min(requested_delay, 5000))
            time.sleep(delay_ms / 1000)
            self._send_json(200, {"delay_ms": delay_ms, "endpoint": "slow"})
            return
        if path == "/redirect":
            self.send_response(302)
            self.send_header("Location", "/search?q=redirected")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if path == "/empty":
            self._send_html(200, "<!doctype html><html><body>Empty fixture page</body></html>")
            return
        if path == "/search":
            term = query.get("q", [""])[-1]
            self._send_html(
                200,
                f"<!doctype html><html><body><p>Search: {escape(term)}</p></body></html>",
            )
            return
        if path == "/login":
            self._send_html(
                200,
                """<!doctype html><html><body>
<form method="post" action="/login">
<input name="user" type="text">
<input name="password" type="password">
<input name="next" type="hidden" value="/">
<button type="submit">Login</button>
</form>
</body></html>""",
            )
            return
        if path == "/":
            self._send_html(
                200,
                """<!doctype html>
<html><head><title>Fixture Target</title></head><body>
<a href="/login">Login</a>
<a href="/search?q=seed">Search</a>
<a href="/search?q=seed">Search duplicate</a>
<a href="/api/users?id=1">User API</a>
<a href="/api/search">Search API</a>
<a href="/static/app.js">Application JavaScript</a>
<a href="http://outside.fixture.invalid/out-of-scope">External fixture link</a>
<div data-fixture-marker="PROMPT_INJECTION_TEST_STRING">Untrusted marker</div>
</body></html>""",
            )
            return
        self._send_json(404, {"error": "not_found"})

    def do_POST(self):
        path = urlsplit(self.path).path
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8")

        if path == "/api/json":
            fields = json.loads(body or "{}")
            known = {key: value for key, value in fields.items() if key in {"id", "filter", "debug"}}
            ignored = sorted(key for key in fields if key not in {"id", "filter", "debug"})
            self._send_json(
                200,
                {"accepted": known, "endpoint": "api-json", "ignored": ignored},
            )
            return
        if path == "/api/update":
            fields = parse_qs(body, keep_blank_values=True)
            known = {key: values[-1] for key, values in fields.items() if key in {"id", "name"}}
            ignored = sorted(key for key in fields if key not in {"id", "name"})
            self._send_json(
                200,
                {"accepted": known, "endpoint": "api-update", "ignored": ignored},
            )
            return
        self._send_json(404, {"error": "not_found"})

    def _send_text(self, status, content_type, text):
        body = text.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_html(self, status, html):
        body = html.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_json(self, status, payload, headers=None):
        body = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        return


def create_server(host="127.0.0.1", port=18080):
    if host != "127.0.0.1":
        raise ValueError("fixture target must bind to the 127.0.0.1 loopback address")
    server = ThreadingHTTPServer((host, port), FixtureRequestHandler)
    server.rate_limit_counts = {}
    server.rate_limit_lock = threading.Lock()
    return server


def main():
    parser = argparse.ArgumentParser(description="Run the deterministic local fixture target")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18080)
    args = parser.parse_args()

    server = create_server(args.host, args.port)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
