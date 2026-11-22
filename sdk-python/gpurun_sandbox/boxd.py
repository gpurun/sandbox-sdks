from __future__ import annotations

import base64
import json
import struct
import threading
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Callable, Dict, Optional, Tuple

from .auth import resolve_access_token, resolve_api_key, resolve_bearer_token, resolve_data_url
from .errors import BoxdError
from .ws import ProcessSocket, WebSocket, normalize_end

_SANDBOX_HEADER = "Novita-Sandbox-Id"
_CONNECT_JSON = "application/connect+json"
_JSON = "application/json"


def _json_bytes(body: Any) -> bytes:
    return json.dumps(body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def _to_error(status: int, text: str, reason: Any = None) -> BoxdError:
    message = text.strip() or (str(reason) if reason else "") or f"HTTP {status}"
    if text:
        try:
            value = json.loads(text)
        except ValueError:
            value = None
        if isinstance(value, dict):
            nested = value.get("error")
            for candidate in (value.get("message"), value.get("error"), nested.get("message") if isinstance(nested, dict) else None, value.get("detail")):
                if isinstance(candidate, str) and candidate:
                    message = candidate
                    break
    return BoxdError(message, status)


def _first_number(body: Dict[str, Any], keys: Tuple[str, ...], default: int) -> int:
    for key in keys:
        value = body.get(key)
        if isinstance(value, bool):
            continue
        if isinstance(value, int):
            return value
        if isinstance(value, str):
            try:
                return int(value.strip())
            except ValueError:
                continue
    return default


def _first_string(body: Dict[str, Any], keys: Tuple[str, ...], default: str) -> str:
    for key in keys:
        value = body.get(key)
        if isinstance(value, str):
            return value
        if value is not None:
            return str(value)
    return default


def _parse_exec_result(text: str, status: int) -> dict:
    body: Dict[str, Any] = {}
    if text:
        try:
            value = json.loads(text)
            if isinstance(value, dict):
                body = dict(value)
        except ValueError:
            body = {}
    result = dict(body)
    result["exitCode"] = _first_number(body, ("exitCode", "exit_code", "code"), 1 if status == 422 else 0)
    if body:
        result["stdout"] = _first_string(body, ("stdout", "output"), "")
        result["stderr"] = _first_string(body, ("stderr", "error"), "")
    else:
        result["stdout"] = text if status < 400 else ""
        result["stderr"] = ""
    return result


def _prepare_send_input(req: Any) -> Any:
    if not isinstance(req, dict):
        return req
    result = dict(req)
    input_value = result.get("input")
    if isinstance(input_value, dict):
        normalized = dict(input_value)
        for key in ("stdin", "pty"):
            value = normalized.get(key)
            if isinstance(value, (bytes, bytearray, memoryview)):
                normalized[key] = base64.b64encode(bytes(value)).decode("ascii")
            elif isinstance(value, str) and value != "":
                normalized[key] = base64.b64encode(value.encode("utf-8")).decode("ascii")
        result["input"] = normalized
    return result


def _read_exact(stream: Any, size: int) -> bytes:
    chunks = []
    remaining = size
    while remaining > 0:
        chunk = stream.read(remaining)
        if not chunk:
            break
        chunks.append(chunk)
        remaining -= len(chunk)
    return b"".join(chunks)


class ProcessStream:
    def __init__(self, response: Any, on_event: Optional[Callable[[dict], None]] = None) -> None:
        self._response = response
        self._on_event = on_event
        self._event = threading.Event()
        self._end: Optional[Dict[str, Any]] = None
        self._error: Optional[Exception] = None
        self._closed = False
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def wait_end(self, timeout: Optional[float] = None) -> dict:
        if not self._event.wait(timeout):
            raise BoxdError("timeout waiting for Connect end frame", 504)
        if self._error is not None:
            raise self._error
        if self._end is None:
            raise BoxdError("Connect end frame unavailable", 502)
        return dict(self._end)

    def close(self) -> None:
        self._closed = True
        try:
            self._response.close()
        except OSError:
            pass
        self._fail(BoxdError("Connect stream closed by client", 499))

    def _run(self) -> None:
        try:
            while not self._closed:
                header = _read_exact(self._response, 5)
                if not header:
                    break
                if len(header) < 5:
                    raise BoxdError("truncated Connect frame header", 502)
                flags = header[0]
                length = struct.unpack(">I", header[1:5])[0]
                payload = _read_exact(self._response, length)
                if len(payload) < length:
                    raise BoxdError("truncated Connect frame payload", 502)
                event: Optional[dict] = None
                if length:
                    try:
                        value = json.loads(payload.decode("utf-8"))
                    except ValueError:
                        raise BoxdError("invalid Connect event frame", 502) from None
                    event = value if isinstance(value, dict) else {}
                    if self._on_event is not None:
                        self._on_event(event)
                wrapped = event.get("event") if isinstance(event, dict) else None
                end_value = wrapped.get("end") if isinstance(wrapped, dict) else None
                if flags & 0x02 or end_value is not None:
                    self._set_end(normalize_end(end_value))
                    return
            if self._end is None:
                self._fail(BoxdError("Connect stream closed before end frame", 502))
        except Exception as error:
            self._fail(error)

    def _set_end(self, end: Dict[str, Any]) -> None:
        if self._end is None:
            self._end = end
            self._event.set()

    def _fail(self, error: Exception) -> None:
        if self._end is not None:
            return
        if self._error is None:
            self._error = error
        self._event.set()


class BoxdClient:
    def __init__(
        self,
        api_key: Optional[str] = None,
        access_token: Optional[str] = None,
        base_url: Optional[str] = None,
        token: Optional[str] = None,
        sandbox_id: Optional[str] = None,
        timeout: Optional[float] = None,
        headers: Optional[dict] = None,
    ) -> None:
        self.base_url = resolve_data_url(base_url).rstrip("/")
        self.sandbox_id = sandbox_id
        self.api_key = resolve_api_key(api_key)
        self.access_token = resolve_access_token(access_token, token)
        self.token = resolve_bearer_token(api_key, access_token, token)
        self.timeout = float(timeout) if timeout is not None and float(timeout) > 0 else None
        self.headers = dict(headers or {})

    def _target(
        self, path: str, sandbox_id: Optional[str] = None, require_sandbox: bool = True
    ) -> Tuple[str, str]:
        value = sandbox_id if sandbox_id is not None else self.sandbox_id
        identifier = value.strip() if isinstance(value, str) else ""
        if require_sandbox and not identifier:
            raise BoxdError("sandboxID is required", 400)
        url = self.base_url + path
        if identifier:
            parts = urllib.parse.urlsplit(url)
            query = urllib.parse.parse_qsl(parts.query, keep_blank_values=True)
            query.append(("sandboxID", identifier))
            url = urllib.parse.urlunsplit(
                (parts.scheme, parts.netloc, parts.path, urllib.parse.urlencode(query), parts.fragment)
            )
        return url, identifier

    def _open(
        self,
        method: str,
        path: str,
        body: Any = None,
        sandbox_id: Optional[str] = None,
        content_type: str = _JSON,
        allowed_statuses: Tuple[int, ...] = (),
        require_sandbox: bool = True,
    ) -> Any:
        url, identifier = self._target(path, sandbox_id, require_sandbox)
        request_headers: dict = {"Accept": content_type}
        if identifier:
            request_headers[_SANDBOX_HEADER] = identifier
        request_headers.update(self.headers)
        request_headers["Content-Type"] = content_type
        data = None
        if body is not None:
            data = body if isinstance(body, (bytes, bytearray)) else _json_bytes(body)
            data = bytes(data)
        if self.token:
            request_headers["Authorization"] = f"Bearer {self.token}"
        request = urllib.request.Request(url, data=data, headers=request_headers, method=method)
        try:
            return urllib.request.urlopen(request, timeout=self.timeout)
        except urllib.error.HTTPError as error:
            if error.code in allowed_statuses:
                return error
            text = error.read().decode("utf-8", "replace")
            error.close()
            raise _to_error(error.code, text, error.reason) from None
        except urllib.error.URLError as error:
            raise BoxdError(str(error.reason), 0) from None
        except OSError as error:
            raise BoxdError(str(error), 0) from None

    def _request_text(
        self,
        method: str,
        path: str,
        body: Any = None,
        sandbox_id: Optional[str] = None,
        content_type: str = _JSON,
        allowed_statuses: Tuple[int, ...] = (),
        require_sandbox: bool = True,
    ) -> Tuple[int, str]:
        response = self._open(method, path, body, sandbox_id, content_type, allowed_statuses, require_sandbox)
        try:
            status = int(response.getcode() or 0)
            text = response.read().decode("utf-8", "replace")
        finally:
            response.close()
        if not (200 <= status < 300) and status not in allowed_statuses:
            raise _to_error(status, text)
        return status, text

    def _json(
        self,
        method: str,
        path: str,
        body: Any = None,
        sandbox_id: Optional[str] = None,
        content_type: str = _JSON,
        require_sandbox: bool = True,
    ) -> Any:
        status, text = self._request_text(method, path, body, sandbox_id, content_type, (), require_sandbox)
        if not text.strip():
            return None
        try:
            return json.loads(text)
        except ValueError:
            raise BoxdError("invalid JSON response", status) from None

    def health(self, sandbox_id: Optional[str] = None) -> dict:
        return self._json("GET", "/healthz", sandbox_id=sandbox_id, require_sandbox=False)

    def exec(
        self,
        cmd: list,
        env_vars: Optional[dict] = None,
        sandbox_id: Optional[str] = None,
    ) -> dict:
        body: dict = {"cmd": cmd}
        if env_vars is not None:
            body["envVars"] = env_vars
        status, text = self._request_text(
            "POST",
            "/exec",
            body,
            sandbox_id=sandbox_id,
            allowed_statuses=(422,),
        )
        return _parse_exec_result(text, status)

    def list_processes(self, sandbox_id: Optional[str] = None) -> list:
        result = self._json("GET", "/processes", sandbox_id=sandbox_id)
        if isinstance(result, list):
            return result
        if isinstance(result, dict):
            processes = result.get("processes")
            return processes if isinstance(processes, list) else []
        return []

    def start_process(self, req: dict, sandbox_id: Optional[str] = None) -> dict:
        result = self._json("POST", "/processes", req, sandbox_id)
        if isinstance(result, dict) and isinstance(result.get("process"), dict):
            return result["process"]
        return result if isinstance(result, dict) else {}

    def wait_process(self, id: str, sandbox_id: Optional[str] = None) -> dict:
        result = self._json(
            "GET",
            f"/processes/{urllib.parse.quote(str(id), safe='')}/wait",
            sandbox_id=sandbox_id,
        )
        if isinstance(result, dict) and isinstance(result.get("process"), dict):
            return result["process"]
        return result if isinstance(result, dict) else {}

    def resize_process(self, id: str, rows: int, cols: int, sandbox_id: Optional[str] = None) -> None:
        self._json(
            "POST",
            f"/processes/{urllib.parse.quote(str(id), safe='')}/resize",
            {"rows": rows, "cols": cols},
            sandbox_id,
        )

    def signal_process(self, id: str, signal: str, sandbox_id: Optional[str] = None) -> None:
        self._json(
            "POST",
            f"/processes/{urllib.parse.quote(str(id), safe='')}/signal",
            {"signal": signal},
            sandbox_id,
        )

    def connect_process(
        self,
        id: str,
        on_output: Optional[Callable[[bytes], None]] = None,
        on_end: Optional[Callable[[dict], None]] = None,
        sandbox_id: Optional[str] = None,
        on_start: Optional[Callable[[dict], None]] = None,
        on_error: Optional[Callable[[Exception], None]] = None,
    ) -> ProcessSocket:
        target, identifier = self._target(
            f"/processes/{urllib.parse.quote(str(id), safe='')}/connect",
            sandbox_id,
        )
        parts = urllib.parse.urlsplit(target)
        scheme = "wss" if parts.scheme in ("https", "wss") else "ws"
        url = urllib.parse.urlunsplit((scheme, parts.netloc, parts.path, parts.query, parts.fragment))
        request_headers = {_SANDBOX_HEADER: identifier}
        request_headers.update(self.headers)
        if self.token:
            request_headers["Authorization"] = f"Bearer {self.token}"
        websocket = WebSocket.connect(url, headers=request_headers, timeout=self.timeout)
        return ProcessSocket(
            websocket,
            on_output=on_output,
            on_end=on_end,
            on_start=on_start,
            on_error=on_error,
        )

    def list_processes_connect(self, sandbox_id: Optional[str] = None) -> list:
        result = self._json(
            "POST",
            "/process.Process/List",
            {},
            sandbox_id,
            _CONNECT_JSON,
        )
        if isinstance(result, dict) and isinstance(result.get("processes"), list):
            return result["processes"]
        return []

    def start_process_stream(
        self,
        req: dict,
        on_event: Optional[Callable[[dict], None]] = None,
        sandbox_id: Optional[str] = None,
    ) -> ProcessStream:
        response = self._open(
            "POST",
            "/process.Process/Start",
            req,
            sandbox_id,
            _CONNECT_JSON,
        )
        return ProcessStream(response, on_event)

    def connect_process_stream(
        self,
        req: dict,
        on_event: Optional[Callable[[dict], None]] = None,
        sandbox_id: Optional[str] = None,
    ) -> ProcessStream:
        response = self._open(
            "POST",
            "/process.Process/Connect",
            req,
            sandbox_id,
            _CONNECT_JSON,
        )
        return ProcessStream(response, on_event)

    def update_process(self, req: dict, sandbox_id: Optional[str] = None) -> None:
        self._json("POST", "/process.Process/Update", req, sandbox_id, _CONNECT_JSON)

    def send_input(self, req: dict, sandbox_id: Optional[str] = None) -> None:
        self._json("POST", "/process.Process/SendInput", _prepare_send_input(req), sandbox_id, _CONNECT_JSON)

    def send_signal(self, req: dict, sandbox_id: Optional[str] = None) -> None:
        self._json("POST", "/process.Process/SendSignal", req, sandbox_id, _CONNECT_JSON)


def decode_data_field(value: Optional[str]) -> Optional[bytes]:
    """Decode a base64 Connect-stream data field (stdout/stderr/pty).

    The Connect envelope is JSON, so these fields are standard base64
    strings; decode before use. Returns None when the field is absent,
    raises binascii.Error (a ValueError) on invalid base64.
    """
    if value is None:
        return None
    return base64.b64decode(value, validate=True)


__all__ = ["BoxdClient", "ProcessStream", "decode_data_field"]
