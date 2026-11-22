import base64
import hashlib
import json
import socket
import struct
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

from gpurun_sandbox import BoxdClient, BoxdError
from gpurun_sandbox.ws import FrameDecoder, encode_frame


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        return

    def do_GET(self):
        self._handle()

    def do_POST(self):
        self._handle()

    def do_PUT(self):
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
            self._write(404, {"message": "no route"}, {"Content-Type": "application/json"})
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
        if payload is not None and not isinstance(payload, bytes) and not any(name.lower() == "content-type" for name in extra):
            self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        for name, value in extra.items():
            self.send_header(name, value)
        self.end_headers()
        if data:
            self.wfile.write(data)


def connect_frame(flags, value):
    payload = json.dumps(value).encode("utf-8")
    return bytes([flags]) + struct.pack(">I", len(payload)) + payload


class BoxdTest(unittest.TestCase):
    def setUp(self):
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.routes = {}
        self.server.requests = []
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.base_url = f"http://127.0.0.1:{self.server.server_address[1]}"
        self.client = BoxdClient(base_url=self.base_url, sandbox_id="sb-1", api_key="data-key")

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)

    def test_exec_success_and_sandbox_identity(self):
        def route(body, handler):
            return (200, {"exitCode": 0, "stdout": "hello\n", "stderr": ""}, {})

        self.server.routes[("POST", "/exec")] = route
        result = self.client.exec(["echo", "hello"], {"A": "B"})
        request = self.server.requests[0]
        self.assertEqual(result, {"exitCode": 0, "stdout": "hello\n", "stderr": ""})
        self.assertEqual(request["body"], {"cmd": ["echo", "hello"], "envVars": {"A": "B"}})
        self.assertEqual(request["headers"].get("Novita-Sandbox-Id"), "sb-1")
        self.assertEqual(request["query"]["sandboxID"], ["sb-1"])
        self.assertEqual(request["headers"].get("Authorization"), "Bearer data-key")

    def test_exec_422_is_not_transport_error(self):
        def route(body, handler):
            return (422, {"exit_code": 7, "stdout": "partial"}, {})

        self.server.routes[("POST", "/exec")] = route
        result = self.client.exec(["false"])
        self.assertEqual(result["exitCode"], 7)
        self.assertEqual(result["stdout"], "partial")
        self.assertEqual(result["stderr"], "")

    def test_exec_400_is_error(self):
        def route(body, handler):
            return (400, b"bad command", {"Content-Type": "text/plain"})

        self.server.routes[("POST", "/exec")] = route
        with self.assertRaises(BoxdError) as raised:
            self.client.exec(["bad"])
        self.assertEqual(raised.exception.status, 400)
        self.assertEqual(str(raised.exception), "bad command")

    def test_sandbox_override(self):
        def route(body, handler):
            return (200, {"processes": [{"pid": 1}]}, {})

        self.server.routes[("GET", "/processes")] = route
        result = self.client.list_processes(sandbox_id="sb-2")
        request = self.server.requests[0]
        self.assertEqual(result, [{"pid": 1}])
        self.assertEqual(request["headers"].get("Novita-Sandbox-Id"), "sb-2")
        self.assertEqual(request["query"]["sandboxID"], ["sb-2"])

    def test_connect_stream_frames(self):
        events = []

        def route(body, handler):
            payload = b"".join(
                [
                    connect_frame(0, {"event": {"start": {"pid": 42}}}),
                    connect_frame(0, {"event": {"data": {"stdout": "aGVsbG8="}}}),
                    connect_frame(0, {"event": {"keepalive": {}}}),
                    connect_frame(2, {"event": {"end": {"exitCode": 3, "exited": True, "status": "exited"}}}),
                ]
            )
            return (200, payload, {"Content-Type": "application/connect+json"})

        self.server.routes[("POST", "/process.Process/Start")] = route
        stream = self.client.start_process_stream({"process": {"cmd": "echo"}}, events.append)
        end = stream.wait_end(timeout=3)
        request = self.server.requests[0]
        self.assertEqual(end, {"exitCode": 3, "exited": True, "status": "exited"})
        self.assertEqual(len(events), 4)
        self.assertEqual(events[0]["event"]["start"]["pid"], 42)
        self.assertEqual(events[1]["event"]["data"]["stdout"], "aGVsbG8=")
        self.assertEqual(events[2]["event"]["keepalive"], {})
        self.assertEqual(events[3]["event"]["end"]["exitCode"], 3)
        self.assertEqual(request["headers"].get("Content-Type"), "application/connect+json")
        self.assertEqual(request["headers"].get("Novita-Sandbox-Id"), "sb-1")
        self.assertEqual(request["query"]["sandboxID"], ["sb-1"])

    def test_connect_unary_and_send_input(self):
        self.server.routes[("POST", "/process.Process/List")] = lambda body, handler: (
            200,
            {"processes": [{"pid": 9}]},
            {"Content-Type": "application/connect+json"},
        )
        self.server.routes[("POST", "/process.Process/SendInput")] = lambda body, handler: (204, None, {})
        self.assertEqual(self.client.list_processes_connect(), [{"pid": 9}])
        self.client.send_input({"process": {"pid": 9}, "input": {"stdin": b"hello"}})
        list_request = self.server.requests[0]
        input_request = self.server.requests[1]
        self.assertEqual(list_request["headers"].get("Content-Type"), "application/connect+json")
        self.assertEqual(input_request["body"], {"process": {"pid": 9}, "input": {"stdin": "aGVsbG8="}})

        self.client.send_input({"process": {"pid": 9}, "input": {"stdin": "hi", "pty": "x"}})
        text_request = self.server.requests[2]
        self.assertEqual(
            text_request["body"],
            {"process": {"pid": 9}, "input": {"stdin": "aGk=", "pty": "eA=="}},
        )

    def test_websocket_frame_encoding_and_decoding(self):
        mask = b"\x01\x02\x03\x04"
        masked = encode_frame(b"hello", mask=True, mask_key=mask)
        self.assertEqual(masked[0], 0x82)
        self.assertEqual(masked[1], 0x85)
        self.assertEqual(masked[2:6], mask)
        self.assertEqual(bytes(byte ^ mask[index % 4] for index, byte in enumerate(masked[6:])), b"hello")
        decoder = FrameDecoder()
        frames = decoder.feed(encode_frame(b"world", opcode=1, mask=False) + encode_frame(b"x" * 200, mask=False))
        self.assertEqual(len(frames), 2)
        self.assertEqual(frames[0].opcode, 1)
        self.assertEqual(frames[0].payload, b"world")
        self.assertEqual(frames[1].opcode, 2)
        self.assertEqual(frames[1].payload, b"x" * 200)

    def test_connect_process_over_websocket(self):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        listener.settimeout(5)
        holder = {}

        def serve():
            connection, _ = listener.accept()
            connection.settimeout(5)
            data = b""
            while b"\r\n\r\n" not in data:
                chunk = connection.recv(4096)
                if not chunk:
                    return
                data += chunk
            request = data.decode("latin-1")
            holder["request"] = request
            key = ""
            for line in request.split("\r\n"):
                if line.lower().startswith("sec-websocket-key:"):
                    key = line.split(":", 1)[1].strip()
            accept = base64.b64encode(hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode("ascii")).digest()).decode("ascii")
            handshake = (
                "HTTP/1.1 101 Switching Protocols\r\n"
                "Upgrade: websocket\r\n"
                "Connection: Upgrade\r\n"
                f"Sec-WebSocket-Accept: {accept}\r\n\r\n"
            )
            start = json.dumps({"type": "start", "process": {"pid": 7}}).encode("utf-8")
            end = json.dumps(
                {
                    "type": "end",
                    "process": {"pid": 0},
                    "exit": {"exitCode": 0, "exited": True, "status": "exited"},
                }
            ).encode("utf-8")
            connection.sendall(
                handshake.encode("latin-1")
                + encode_frame(start, opcode=1, mask=False)
                + encode_frame(b"hello", opcode=2, mask=False)
                + encode_frame(end, opcode=1, mask=False)
            )
            time.sleep(0.2)
            connection.close()

        server_thread = threading.Thread(target=serve, daemon=True)
        server_thread.start()
        client = BoxdClient(base_url=f"http://127.0.0.1:{listener.getsockname()[1]}", sandbox_id="sb-ws")
        outputs = []
        socket_handle = client.connect_process("p1", on_output=outputs.append)
        end = socket_handle.wait_end(timeout=3)
        socket_handle.close()
        server_thread.join(timeout=3)
        listener.close()
        self.assertEqual(outputs, [b"hello"])
        self.assertEqual(end, {"exitCode": 0, "exited": True, "status": "exited"})
        self.assertIn("GET /processes/p1/connect?sandboxID=sb-ws", holder["request"])
        self.assertIn("Novita-Sandbox-Id: sb-ws", holder["request"])
        self.assertIn("Sec-WebSocket-Version: 13", holder["request"])

    def test_connect_process_tolerates_binary_event_frames(self):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        listener.settimeout(5)

        def serve():
            connection, _ = listener.accept()
            connection.settimeout(5)
            data = b""
            while b"\r\n\r\n" not in data:
                chunk = connection.recv(4096)
                if not chunk:
                    return
                data += chunk
            request = data.decode("latin-1")
            key = ""
            for line in request.split("\r\n"):
                if line.lower().startswith("sec-websocket-key:"):
                    key = line.split(":", 1)[1].strip()
            accept = base64.b64encode(hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode("ascii")).digest()).decode("ascii")
            handshake = (
                "HTTP/1.1 101 Switching Protocols\r\n"
                "Upgrade: websocket\r\n"
                "Connection: Upgrade\r\n"
                f"Sec-WebSocket-Accept: {accept}\r\n\r\n"
            )
            start = json.dumps({"type": "start", "process": {"pid": 7}}).encode("utf-8")
            end = json.dumps(
                {
                    "type": "end",
                    "process": {"pid": 0},
                    "exit": {"exitCode": 3, "exited": True, "status": "exited"},
                }
            ).encode("utf-8")
            connection.sendall(
                handshake.encode("latin-1")
                + encode_frame(start, opcode=2, mask=False)
                + encode_frame(b"hello", opcode=2, mask=False)
                + encode_frame(end, opcode=2, mask=False)
            )
            time.sleep(0.2)
            connection.close()

        server_thread = threading.Thread(target=serve, daemon=True)
        server_thread.start()
        client = BoxdClient(base_url=f"http://127.0.0.1:{listener.getsockname()[1]}", sandbox_id="sb-ws")
        outputs = []
        starts = []
        socket_handle = client.connect_process(
            "p1", on_output=outputs.append, on_start=starts.append
        )
        end = socket_handle.wait_end(timeout=3)
        socket_handle.close()
        server_thread.join(timeout=3)
        listener.close()
        self.assertEqual(outputs, [b"hello"])
        self.assertEqual(starts, [{"pid": 7}])
        self.assertEqual(end, {"exitCode": 3, "exited": True, "status": "exited"})


    def test_decode_data_field(self):
        from gpurun_sandbox import decode_data_field

        self.assertEqual(decode_data_field("aGVsbG8="), b"hello")
        self.assertIsNone(decode_data_field(None))
        with self.assertRaises(ValueError):
            decode_data_field("!!!")


if __name__ == "__main__":
    unittest.main()
