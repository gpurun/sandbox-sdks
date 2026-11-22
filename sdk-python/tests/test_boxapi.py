import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

from gpurun_sandbox import BoxApiClient, BoxApiError, Gpurun


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        return

    def do_GET(self):
        self._handle()

    def do_POST(self):
        self._handle()

    def do_PUT(self):
        self._handle()

    def do_PATCH(self):
        self._handle()

    def do_DELETE(self):
        self._handle()

    def _handle(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        body = None
        if raw:
            try:
                body = json.loads(raw.decode("utf-8"))
            except ValueError:
                body = raw.decode("utf-8", "replace")
        parsed = urlsplit(self.path)
        self.server.requests.append(
            {
                "method": self.command,
                "path": parsed.path,
                "query": parse_qs(parsed.query),
                "headers": dict(self.headers),
                "body": body,
            }
        )
        route = self.server.routes.get((self.command, parsed.path))
        if route is None:
            self._write(404, {"code": "not_found", "message": "no route"}, {})
            return
        status, payload, headers = route(body, self)
        self._write(status, payload, headers)

    def _write(self, status, payload, headers):
        if isinstance(payload, bytes):
            data = payload
        elif payload is None:
            data = b""
        else:
            data = json.dumps(payload).encode("utf-8")
        extra = dict(headers or {})
        self.send_response(status)
        if payload is not None and not isinstance(payload, bytes) and not any(
            name.lower() == "content-type" for name in extra
        ):
            self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        for name, value in extra.items():
            self.send_header(name, value)
        self.end_headers()
        if data:
            self.wfile.write(data)


class BoxApiTest(unittest.TestCase):
    def setUp(self):
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.routes = {}
        self.server.requests = []
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base_url = f"http://127.0.0.1:{self.server.server_address[1]}"
        self.client = BoxApiClient(base_url=self.base_url, api_key="secret")

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)

    def test_auth_header_and_health(self):
        self.server.routes[("GET", "/health")] = lambda body, handler: (200, {"status": "ok"}, {})
        result = self.client.health()
        request = self.server.requests[0]
        self.assertEqual(result, {"status": "ok"})
        self.assertEqual(request["headers"].get("Authorization"), "Bearer secret")
        self.assertEqual(request["headers"].get("Accept"), "application/json")

    def test_create_sandbox(self):
        received = {}

        def route(body, handler):
            received.update(body)
            return (200, {"sandboxID": "sb-1", "templateID": body["templateID"]}, {})

        self.server.routes[("POST", "/sandboxes")] = route
        result = self.client.create_sandbox({"templateID": "tpl-1", "timeout": 60})
        self.assertEqual(result["sandboxID"], "sb-1")
        self.assertEqual(received, {"templateID": "tpl-1", "timeout": 60})
        with self.assertRaises(BoxApiError) as raised:
            self.client.create_sandbox({})
        self.assertEqual(raised.exception.status, 400)
        self.assertEqual(raised.exception.code, "invalid_argument")

    def test_list_pagination(self):
        def route(body, handler):
            return (200, [{"sandboxID": "sb-1"}], {"X-Next-Token": "header-token"})

        self.server.routes[("GET", "/sandboxes")] = route
        result = self.client.list_sandboxes_v2(state=["running", "paused"], next_token="5", limit=5000)
        self.assertEqual(result, {"items": [{"sandboxID": "sb-1"}], "next_token": "header-token"})
        request = self.server.requests[0]
        self.assertEqual(request["query"]["state"], ["running", "paused"])
        self.assertEqual(request["query"]["nextToken"], ["5"])
        self.assertEqual(request["query"]["limit"], ["1000"])
        with self.assertRaises(BoxApiError):
            self.client.list_sandboxes_v2(next_token="abc")
        with self.assertRaises(BoxApiError):
            self.client.list_sandboxes_v2(next_token=-1)

    def test_list_pagination_accepts_items_envelope(self):
        def route(body, handler):
            return (200, {"items": [{"sandboxID": "sb-2"}], "nextToken": "body-token"}, {})

        self.server.routes[("GET", "/sandboxes")] = route
        result = self.client.list_sandboxes_v2()
        self.assertEqual(result, {"items": [{"sandboxID": "sb-2"}], "next_token": "body-token"})

    def test_error_mapping(self):
        self.server.routes[("GET", "/v1/sandboxes")] = lambda body, handler: (
            404,
            {"code": "sandbox_not_found", "message": "missing sandbox"},
            {},
        )
        with self.assertRaises(BoxApiError) as raised:
            self.client.list_sandboxes()
        self.assertEqual(raised.exception.status, 404)
        self.assertEqual(raised.exception.code, "sandbox_not_found")
        self.assertEqual(str(raised.exception), "missing sandbox")
        self.server.routes[("GET", "/v1/scheduler/quotas")] = (
            lambda body, handler: (500, b"boom", {"Content-Type": "text/plain"})
        )
        with self.assertRaises(BoxApiError) as raised:
            self.client.get_quotas()
        self.assertEqual(raised.exception.status, 500)
        self.assertEqual(raised.exception.code, "internal_error")
        self.assertEqual(str(raised.exception), "boom")

    def test_decide_uses_pascal_case(self):
        received = {}

        def route(body, handler):
            received.update(body)
            return (200, {"provider": "local", "strategy": "balanced"}, {})

        self.server.routes[("POST", "/v1/scheduler/decide")] = route
        result = self.client.decide(
            {
                "sandboxID": "sb-1",
                "templateID": "tpl-1",
                "imageID": "img-1",
                "labels": {"team": "sdk"},
                "annotations": {"a": "b"},
                "region": "eu",
                "providerHint": "local",
                "requirements": {"minCPU": 2, "minMemory": 1024, "needGPU": True, "region": "eu"},
            }
        )
        self.assertEqual(result["provider"], "local")
        self.assertEqual(
            received,
            {
                "SandboxID": "sb-1",
                "TemplateID": "tpl-1",
                "ImageID": "img-1",
                "Labels": {"team": "sdk"},
                "Annotations": {"a": "b"},
                "Region": "eu",
                "ProviderHint": "local",
                "Requirements": {"MinCPU": 2, "MinMemory": 1024, "NeedGPU": True, "Region": "eu"},
            },
        )

    def test_route_matrix(self):
        calls = [
            (lambda: self.client.get_sandbox("sb1"), "GET", "/sandboxes/sb1"),
            (lambda: self.client.kill_sandbox("sb1"), "DELETE", "/sandboxes/sb1"),
            (lambda: self.client.pause_sandbox("sb1"), "POST", "/v1/sandboxes/sb1/pause"),
            (lambda: self.client.resume_sandbox("sb1"), "POST", "/v1/sandboxes/sb1/resume"),
            (lambda: self.client.connect_sandbox("sb1"), "POST", "/sandboxes/sb1/connect"),
            (lambda: self.client.set_sandbox_timeout("sb1"), "POST", "/sandboxes/sb1/timeout"),
            (lambda: self.client.poweroff_sandbox("sb1"), "POST", "/v1/sandboxes/sb1/poweroff"),
            (lambda: self.client.poweron_sandbox("sb1"), "POST", "/v1/sandboxes/sb1/poweron"),
            (lambda: self.client.reboot_sandbox("sb1"), "POST", "/v1/sandboxes/sb1/reboot"),
            (lambda: self.client.get_balloon("sb1"), "GET", "/v1/sandboxes/sb1/balloon"),
            (lambda: self.client.update_balloon("sb1", 512), "PATCH", "/v1/sandboxes/sb1/balloon"),
            (lambda: self.client.get_balloon_stats("sb1"), "GET", "/v1/sandboxes/sb1/balloon/statistics"),
            (lambda: self.client.update_balloon_stats("sb1", 15), "PATCH", "/v1/sandboxes/sb1/balloon/statistics"),
            (lambda: self.client.get_balloon_hinting("sb1"), "GET", "/v1/sandboxes/sb1/balloon/hinting"),
            (lambda: self.client.start_balloon_hinting("sb1"), "POST", "/v1/sandboxes/sb1/balloon/hinting/start"),
            (lambda: self.client.stop_balloon_hinting("sb1"), "POST", "/v1/sandboxes/sb1/balloon/hinting/stop"),
            (lambda: self.client.list_templates(), "GET", "/templates"),
            (lambda: self.client.get_template("tpl1"), "GET", "/templates/tpl1"),
            (lambda: self.client.delete_template("tpl1"), "DELETE", "/templates/tpl1"),
            (lambda: self.client.get_template_build_status("tpl1", "b1"), "GET", "/templates/tpl1/builds/b1/status"),
            (lambda: self.client.start_template_build_v2("tpl1", "b1", {}), "POST", "/v2/templates/tpl1/builds/b1"),
            (lambda: self.client.create_template_v3({}), "POST", "/v3/templates"),
            (lambda: self.client.convert_template({}), "POST", "/v1/templates/convert"),
            (lambda: self.client.create_image({"templateID": "tpl1"}), "POST", "/v1/images"),
            (lambda: self.client.list_images(), "GET", "/v1/images"),
            (lambda: self.client.get_image("img1"), "GET", "/v1/images/img1"),
            (lambda: self.client.delete_image("img1"), "DELETE", "/v1/images/img1"),
            (lambda: self.client.list_runtimes(), "GET", "/v1/runtimes"),
            (lambda: self.client.get_runtime("firecracker"), "GET", "/v1/runtimes/firecracker"),
            (
                lambda: self.client.get_runtime_capabilities("firecracker"),
                "GET",
                "/v1/runtimes/firecracker/capabilities",
            ),
            (lambda: self.client.list_strategies(), "GET", "/v1/scheduler/strategies"),
            (lambda: self.client.set_strategy({}), "POST", "/v1/scheduler/strategies"),
            (lambda: self.client.get_strategy("s1"), "GET", "/v1/scheduler/strategies/s1"),
            (lambda: self.client.delete_strategy("s1"), "DELETE", "/v1/scheduler/strategies/s1"),
            (lambda: self.client.get_default_strategy(), "GET", "/v1/scheduler/strategies/default"),
            (lambda: self.client.set_default_strategy("s1"), "PUT", "/v1/scheduler/strategies/default"),
            (lambda: self.client.update_quota({}), "PUT", "/v1/scheduler/quotas"),
        ]
        for call, method, path in calls:
            self.server.routes[(method, path)] = lambda body, handler: (200, {"ok": True}, {})
        for call, method, path in calls:
            call()
            request = self.server.requests[-1]
            self.assertEqual((request["method"], request["path"]), (method, path))

    def test_facade_construction(self):
        facade = Gpurun(
            api_key="facade-key",
            base_url=self.base_url + "/control",
            data_url=self.base_url + "/data",
            sandbox_id="sb-1",
            timeout=3,
        )
        self.assertEqual(facade.box.base_url, self.base_url + "/control")
        self.assertEqual(facade.data.base_url, self.base_url + "/data")
        self.assertEqual(facade.data.sandbox_id, "sb-1")
        self.assertEqual(facade.box.token, "facade-key")


if __name__ == "__main__":
    unittest.main()
