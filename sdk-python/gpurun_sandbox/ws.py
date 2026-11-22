from __future__ import annotations

import base64
import hashlib
import json
import os
import socket
import ssl
import struct
import threading
import urllib.parse
from typing import Any, Callable, Dict, List, Optional, Tuple

from .errors import BoxdError

_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
_OP_CONTINUATION = 0x0
_OP_TEXT = 0x1
_OP_BINARY = 0x2
_OP_CLOSE = 0x8
_OP_PING = 0x9
_OP_PONG = 0xA


class WebSocketFrame:
    def __init__(self, fin: bool, opcode: int, payload: bytes) -> None:
        self.fin = fin
        self.opcode = opcode
        self.payload = payload


class FrameDecoder:
    def __init__(self) -> None:
        self._buffer = bytearray()

    def feed(self, data: bytes) -> List[WebSocketFrame]:
        self._buffer.extend(data)
        frames: List[WebSocketFrame] = []
        while True:
            parsed = self._try_parse()
            if parsed is None:
                return frames
            frame, consumed = parsed
            del self._buffer[:consumed]
            frames.append(frame)

    def _try_parse(self) -> Optional[Tuple[WebSocketFrame, int]]:
        if len(self._buffer) < 2:
            return None
        first = self._buffer[0]
        second = self._buffer[1]
        fin = bool(first & 0x80)
        opcode = first & 0x0F
        masked = bool(second & 0x80)
        length = second & 0x7F
        offset = 2
        if length == 126:
            if len(self._buffer) < offset + 2:
                return None
            length = struct.unpack_from(">H", self._buffer, offset)[0]
            offset += 2
        elif length == 127:
            if len(self._buffer) < offset + 8:
                return None
            length = struct.unpack_from(">Q", self._buffer, offset)[0]
            offset += 8
        mask = b""
        if masked:
            if len(self._buffer) < offset + 4:
                return None
            mask = bytes(self._buffer[offset : offset + 4])
            offset += 4
        if len(self._buffer) < offset + length:
            return None
        payload = bytes(self._buffer[offset : offset + length])
        if mask:
            payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
        return WebSocketFrame(fin, opcode, payload), offset + length


def encode_frame(
    payload: Any,
    opcode: int = _OP_BINARY,
    fin: bool = True,
    mask: bool = True,
    mask_key: Optional[bytes] = None,
) -> bytes:
    if isinstance(payload, str):
        data = payload.encode("utf-8")
    elif isinstance(payload, (bytes, bytearray, memoryview)):
        data = bytes(payload)
    else:
        raise TypeError("payload must be bytes or str")
    length = len(data)
    header = bytearray()
    header.append((0x80 if fin else 0) | (opcode & 0x0F))
    second = length
    if length >= 126 and length <= 0xFFFF:
        second = 126
    elif length > 0xFFFF:
        second = 127
    if mask:
        second |= 0x80
    header.append(second)
    if second & 0x7F == 126:
        header.extend(struct.pack(">H", length))
    elif second & 0x7F == 127:
        header.extend(struct.pack(">Q", length))
    if not mask:
        return bytes(header) + data
    key = mask_key if mask_key is not None else os.urandom(4)
    if len(key) != 4:
        raise ValueError("mask_key must be exactly 4 bytes")
    masked = bytes(byte ^ key[index % 4] for index, byte in enumerate(data))
    return bytes(header) + key + masked


def _first_int(value: Dict[str, Any], keys: Tuple[str, ...], default: int = 0) -> int:
    for key in keys:
        item = value.get(key)
        if isinstance(item, bool):
            continue
        if isinstance(item, int):
            return item
        if isinstance(item, str):
            try:
                return int(item.strip())
            except ValueError:
                continue
    return default


def _first_bool(value: Dict[str, Any], keys: Tuple[str, ...], default: bool = True) -> bool:
    for key in keys:
        item = value.get(key)
        if isinstance(item, bool):
            return item
    return default


def _first_string(value: Dict[str, Any], keys: Tuple[str, ...], default: str = "") -> str:
    for key in keys:
        item = value.get(key)
        if isinstance(item, str):
            return item
        if item is not None:
            return str(item)
    return default


def normalize_end(value: Any) -> Dict[str, Any]:
    source = value if isinstance(value, dict) else {}
    nested = source.get("exit")
    exit_value = nested if isinstance(nested, dict) else source
    result: Dict[str, Any] = {
        "exitCode": _first_int(exit_value, ("exitCode", "exit_code", "code")),
        "exited": _first_bool(exit_value, ("exited",)),
        "status": _first_string(exit_value, ("status",)),
    }
    for key in ("error", "message"):
        item = exit_value.get(key)
        if isinstance(item, str) and item:
            result["error"] = item
            break
    return result


_WS_EVENT_TYPES = ("start", "data", "end", "keepalive")


def _probe_ws_event(payload: bytes) -> Optional[Dict[str, Any]]:
    try:
        value = json.loads(payload.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        return None
    if isinstance(value, dict) and value.get("type") in _WS_EVENT_TYPES:
        return value
    return None


def _read_http_head(connection: socket.socket) -> Tuple[str, bytes]:
    data = bytearray()
    while b"\r\n\r\n" not in data:
        if len(data) > 65536:
            raise BoxdError("WebSocket handshake header too large", 502)
        chunk = connection.recv(4096)
        if not chunk:
            raise BoxdError("WebSocket handshake closed", 502)
        data.extend(chunk)
    index = data.find(b"\r\n\r\n")
    head = bytes(data[:index]).decode("latin-1")
    rest = bytes(data[index + 4 :])
    return head, rest


class WebSocket:
    def __init__(self, connection: socket.socket, key: str) -> None:
        self._connection = connection
        self._key = key
        self._decoder = FrameDecoder()
        self._messages: List[Tuple[int, bytes]] = []
        self._fragments: List[bytes] = []
        self._fragment_opcode: Optional[int] = None
        self._closed = False
        self._send_lock = threading.Lock()

    @classmethod
    def connect(
        cls,
        url: str,
        headers: Optional[Dict[str, str]] = None,
        timeout: Optional[float] = None,
    ) -> "WebSocket":
        parts = urllib.parse.urlsplit(url)
        host = parts.hostname
        if not host:
            raise BoxdError("invalid WebSocket URL", 400)
        secure = parts.scheme in ("https", "wss")
        port = parts.port or (443 if secure else 80)
        connection = socket.create_connection((host, port), timeout=timeout)
        if secure:
            connection = ssl.create_default_context().wrap_socket(connection, server_hostname=host)
        key = base64.b64encode(os.urandom(16)).decode("ascii")
        path = parts.path or "/"
        if parts.query:
            path += "?" + parts.query
        lines = [
            f"GET {path} HTTP/1.1",
            f"Host: {parts.netloc or host}",
            "Upgrade: websocket",
            "Connection: Upgrade",
            f"Sec-WebSocket-Key: {key}",
            "Sec-WebSocket-Version: 13",
        ]
        for name, value in (headers or {}).items():
            lines.append(f"{name}: {value}")
        handshake = "\r\n".join(lines) + "\r\n\r\n"
        try:
            connection.sendall(handshake.encode("latin-1"))
            head, rest = _read_http_head(connection)
            status_line = head.split("\r\n", 1)[0]
            status_parts = status_line.split(" ", 2)
            status = int(status_parts[1]) if len(status_parts) > 1 else 0
            if status != 101:
                raise BoxdError(f"WebSocket handshake failed: {status_line}", 502)
            accept = None
            for line in head.split("\r\n")[1:]:
                if ":" in line:
                    name, value = line.split(":", 1)
                    if name.strip().lower() == "sec-websocket-accept":
                        accept = value.strip()
            expected = base64.b64encode(hashlib.sha1((key + _GUID).encode("ascii")).digest()).decode("ascii")
            if accept != expected:
                raise BoxdError("WebSocket handshake accept mismatch", 502)
            websocket = cls(connection, key)
            if rest:
                websocket._handle_data(rest)
            return websocket
        except Exception:
            connection.close()
            raise

    def send(self, data: Any, opcode: int = _OP_BINARY, fin: bool = True) -> None:
        if self._closed:
            return
        frame = encode_frame(data, opcode=opcode, fin=fin, mask=True)
        with self._send_lock:
            self._connection.sendall(frame)

    def recv(self) -> Optional[Tuple[int, bytes]]:
        while True:
            if self._messages:
                opcode, payload = self._messages.pop(0)
                if opcode == _OP_CLOSE:
                    return None
                return opcode, payload
            if self._closed:
                return None
            chunk = self._connection.recv(65536)
            if not chunk:
                self._closed = True
                return None
            self._handle_data(chunk)

    def close(self) -> None:
        with self._send_lock:
            if self._closed:
                return
            self._closed = True
            try:
                self._connection.sendall(encode_frame(b"", opcode=_OP_CLOSE, mask=True))
            except OSError:
                pass
            try:
                self._connection.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            self._connection.close()

    def _handle_data(self, data: bytes) -> None:
        for frame in self._decoder.feed(data):
            opcode = frame.opcode
            if opcode == _OP_PING:
                self.send(frame.payload, opcode=_OP_PONG)
                continue
            if opcode == _OP_PONG:
                continue
            if opcode == _OP_CLOSE:
                self._closed = True
                self._messages.append((_OP_CLOSE, b""))
                continue
            if opcode in (_OP_TEXT, _OP_BINARY):
                self._fragment_opcode = opcode
                self._fragments = [frame.payload]
            elif opcode == _OP_CONTINUATION and self._fragment_opcode is not None:
                self._fragments.append(frame.payload)
            else:
                continue
            if frame.fin:
                message = b"".join(self._fragments)
                self._messages.append((self._fragment_opcode, message))
                self._fragment_opcode = None
                self._fragments = []


class ProcessSocket:
    def __init__(
        self,
        websocket: WebSocket,
        on_output: Optional[Callable[[bytes], None]] = None,
        on_end: Optional[Callable[[Dict[str, Any]], None]] = None,
        on_start: Optional[Callable[[Dict[str, Any]], None]] = None,
        on_error: Optional[Callable[[Exception], None]] = None,
    ) -> None:
        self._websocket = websocket
        self._on_output = on_output
        self._on_end = on_end
        self._on_start = on_start
        self._on_error = on_error
        self._event = threading.Event()
        self._end: Optional[Dict[str, Any]] = None
        self._error: Optional[Exception] = None
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def send(self, data: Any) -> None:
        payload = data.encode("utf-8") if isinstance(data, str) else data
        self._websocket.send(payload, opcode=_OP_BINARY)

    def close(self) -> None:
        self._websocket.close()

    def wait_end(self, timeout: Optional[float] = None) -> Dict[str, Any]:
        if not self._event.wait(timeout):
            raise BoxdError("timeout waiting for process end", 504)
        if self._error is not None:
            raise self._error
        if self._end is None:
            raise BoxdError("process end unavailable", 502)
        return dict(self._end)

    def _run(self) -> None:
        try:
            while True:
                message = self._websocket.recv()
                if message is None:
                    if self._end is None:
                        self._fail(BoxdError("WebSocket closed before end frame", 502))
                    return
                opcode, payload = message
                if opcode == _OP_TEXT:
                    try:
                        value = json.loads(payload.decode("utf-8"))
                    except ValueError:
                        self._fail(BoxdError("invalid WebSocket JSON frame", 502))
                        return
                    if not isinstance(value, dict):
                        continue
                    if self._handle_event(value):
                        return
                elif opcode == _OP_BINARY:
                    # boxproxy 会把 boxd 的 TEXT 事件帧改写成 BINARY，
                    # 所以 BINARY 帧要先探测是否为 JSON 事件，否则才是 stdout 数据。
                    value = _probe_ws_event(payload)
                    if value is None:
                        if self._on_output is not None:
                            self._on_output(payload)
                        continue
                    if self._handle_event(value):
                        return
        except Exception as error:
            self._fail(error)

    def _handle_event(self, value: Dict[str, Any]) -> bool:
        message_type = value.get("type")
        if message_type == "start":
            if self._on_start is not None:
                process = value.get("process")
                self._on_start(process if isinstance(process, dict) else value)
        elif message_type == "end":
            end = normalize_end(value.get("exit"))
            self._set_end(end)
            if self._on_end is not None:
                self._on_end(end)
            return True
        return False

    def _set_end(self, end: Dict[str, Any]) -> None:
        if self._end is None:
            self._end = end
            self._event.set()

    def _fail(self, error: Exception) -> None:
        if self._end is not None:
            return
        self._error = error
        if self._on_error is not None:
            self._on_error(error)
        self._event.set()


__all__ = [
    "WebSocketFrame",
    "FrameDecoder",
    "WebSocket",
    "ProcessSocket",
    "encode_frame",
    "normalize_end",
]
