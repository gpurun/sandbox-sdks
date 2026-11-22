from __future__ import annotations

import json
import math
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Mapping
from typing import Any, Optional, Sequence, Tuple

from .auth import resolve_access_token, resolve_api_key, resolve_base_url, resolve_bearer_token
from .errors import BoxApiError

_JSON = "application/json"


def _error_code(status: int) -> str:
    if status == 400:
        return "bad_request"
    if status == 401:
        return "unauthorized"
    if status == 403:
        return "forbidden"
    if status == 404:
        return "not_found"
    if status == 409:
        return "conflict"
    if status == 422:
        return "unprocessable_entity"
    if status >= 500:
        return "internal_error"
    return "http_error"


def _json_bytes(body: Any) -> bytes:
    return json.dumps(body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def _segment(value: Any) -> str:
    return urllib.parse.quote(str(value), safe="")


def _non_negative_int(value: Any, name: str) -> int:
    if isinstance(value, bool):
        raise BoxApiError(f"{name} must be a non-negative number", 400, "invalid_argument")
    if isinstance(value, int):
        result = value
    elif isinstance(value, float):
        if not math.isfinite(value) or not value.is_integer():
            raise BoxApiError(f"{name} must be a non-negative number", 400, "invalid_argument")
        result = int(value)
    elif isinstance(value, str):
        text = value.strip()
        if not text:
            raise BoxApiError(f"{name} must be a non-negative number", 400, "invalid_argument")
        try:
            result = int(text)
        except ValueError:
            raise BoxApiError(f"{name} must be a non-negative number", 400, "invalid_argument") from None
    else:
        raise BoxApiError(f"{name} must be a non-negative number", 400, "invalid_argument")
    if result < 0:
        raise BoxApiError(f"{name} must be a non-negative number", 400, "invalid_argument")
    return result


def _to_error(status: int, text: str, reason: Any = None) -> BoxApiError:
    code = _error_code(status)
    message = text.strip() or (str(reason) if reason else "") or f"HTTP {status}"
    if text:
        try:
            value = json.loads(text)
        except ValueError:
            value = None
        if isinstance(value, dict):
            response_code = value.get("code")
            response_message = value.get("message")
            if isinstance(response_code, str) and response_code:
                code = response_code
            if isinstance(response_message, str) and response_message:
                message = response_message
    return BoxApiError(message, status, code)


def _assert_create_request(req: Any) -> None:
    if not isinstance(req, Mapping):
        raise BoxApiError("create sandbox request is required", 400, "invalid_argument")
    has_source = False
    for key in ("templateID", "imageID", "image_id", "snapshotID", "snapshot_id", "rootfs"):
        value = req.get(key)
        if value is not None and value != "":
            has_source = True
    if not has_source:
        raise BoxApiError(
            "one of templateID, imageID, snapshotID or rootfs is required",
            400,
            "invalid_argument",
        )
    if "rootfs" in req and req.get("rootfs") is not None:
        rootfs = req.get("rootfs")
        if not isinstance(rootfs, Mapping) or rootfs.get("provider") != "overlaybd":
            raise BoxApiError('rootfs must be {"provider": "overlaybd", "image": string}', 400, "invalid_argument")
        image = rootfs.get("image")
        if not isinstance(image, str) or not image:
            raise BoxApiError('rootfs must be {"provider": "overlaybd", "image": string}', 400, "invalid_argument")


_CREATE_KEY_MAP = (("imageID", "image_id"), ("snapshotID", "snapshot_id"), ("runtimeType", "runtime_type"))


def _create_sandbox_wire(req: Mapping) -> dict:
    payload = dict(req)
    for camel, wire_key in _CREATE_KEY_MAP:
        if camel in payload and wire_key not in payload:
            payload[wire_key] = payload.pop(camel)
    return payload


def _pascal_decide_request(req: Any) -> dict:
    source = req if isinstance(req, Mapping) else {}
    payload: dict = {}

    def copy_key(lower: str, upper: str) -> None:
        value = source.get(lower)
        if value is None:
            value = source.get(upper)
        if value is not None:
            payload[upper] = value

    for lower, upper in (
        ("sandboxID", "SandboxID"),
        ("templateID", "TemplateID"),
        ("imageID", "ImageID"),
        ("labels", "Labels"),
        ("annotations", "Annotations"),
        ("region", "Region"),
        ("providerHint", "ProviderHint"),
    ):
        copy_key(lower, upper)

    requirements = source.get("requirements")
    if requirements is None:
        requirements = source.get("Requirements")
    if isinstance(requirements, Mapping):
        nested: dict = {}

        def copy_requirement(lower: str, upper: str) -> None:
            value = requirements.get(lower)
            if value is None:
                value = requirements.get(upper)
            if value is not None:
                nested[upper] = value

        for lower, upper in (
            ("minCPU", "MinCPU"),
            ("minMemory", "MinMemory"),
            ("needGPU", "NeedGPU"),
            ("region", "Region"),
        ):
            copy_requirement(lower, upper)
        if nested:
            payload["Requirements"] = nested
    return payload


class BoxApiClient:
    def __init__(
        self,
        api_key: Optional[str] = None,
        access_token: Optional[str] = None,
        base_url: Optional[str] = None,
        token: Optional[str] = None,
        timeout: Optional[float] = None,
        headers: Optional[dict] = None,
    ) -> None:
        self.base_url = resolve_base_url(base_url).rstrip("/")
        self.api_key = resolve_api_key(api_key)
        self.access_token = resolve_access_token(access_token, token)
        self.token = resolve_bearer_token(api_key, access_token, token)
        self.timeout = float(timeout) if timeout is not None and float(timeout) > 0 else None
        self.headers = dict(headers or {})

    def _request(
        self,
        method: str,
        path: str,
        body: Any = None,
        query: Optional[Sequence[Tuple[str, str]]] = None,
    ) -> Tuple[Any, Any]:
        url = self.base_url + path
        if query:
            encoded = urllib.parse.urlencode(list(query))
            if encoded:
                url += "?" + encoded
        request_headers: dict = {"Accept": _JSON}
        request_headers.update(self.headers)
        data = None
        if body is not None:
            request_headers["Content-Type"] = _JSON
            data = _json_bytes(body)
        if self.token:
            request_headers["Authorization"] = f"Bearer {self.token}"
        request = urllib.request.Request(url, data=data, headers=request_headers, method=method)
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                status = int(response.getcode() or 0)
                text = response.read().decode("utf-8", "replace")
                response_headers = response.headers
        except urllib.error.HTTPError as error:
            text = error.read().decode("utf-8", "replace")
            error.close()
            raise _to_error(error.code, text, error.reason) from None
        except urllib.error.URLError as error:
            raise BoxApiError(str(error.reason), 0, "network_error") from None
        except OSError as error:
            raise BoxApiError(str(error), 0, "network_error") from None
        if status < 200 or status >= 300:
            raise _to_error(status, text)
        if not text.strip():
            return None, response_headers
        try:
            return json.loads(text), response_headers
        except ValueError:
            raise BoxApiError("invalid JSON response", status, "invalid_response") from None

    def _call(
        self,
        method: str,
        path: str,
        body: Any = None,
        query: Optional[Sequence[Tuple[str, str]]] = None,
    ) -> Any:
        return self._request(method, path, body, query)[0]

    def health(self) -> dict:
        return self._call("GET", "/health")

    def create_sandbox(self, req: dict) -> dict:
        _assert_create_request(req)
        return self._call("POST", "/sandboxes", _create_sandbox_wire(req))

    def list_sandboxes(self) -> list:
        return self._call("GET", "/v1/sandboxes")

    def list_sandboxes_v2(
        self,
        state: Any = None,
        next_token: Any = 0,
        limit: Any = 100,
    ) -> dict:
        offset = _non_negative_int(next_token, "nextToken")
        limit_value = _non_negative_int(100 if limit is None else limit, "limit")
        if limit_value > 1000:
            limit_value = 1000
        if state is None:
            states: list = []
        elif isinstance(state, (str, bytes)):
            states = [state]
        else:
            try:
                states = list(state)
            except TypeError:
                states = [state]
        query = [("state", str(item)) for item in states]
        query.extend((("nextToken", str(offset)), ("limit", str(limit_value))))
        data, response_headers = self._request("GET", "/sandboxes", query=query)
        token = None
        if response_headers is not None:
            token = response_headers.get("X-Next-Token")
        if token is None or token == "":
            if isinstance(data, dict):
                token = data.get("nextToken")
                if token is None:
                    token = data.get("next_token")
        if isinstance(data, list):
            items = data
        elif isinstance(data, dict):
            items = data.get("items")
        else:
            items = None
        return {"items": items if isinstance(items, list) else [], "next_token": token}

    def get_sandbox(self, id: str) -> dict:
        return self._call("GET", f"/sandboxes/{_segment(id)}")

    def kill_sandbox(self, id: str) -> None:
        self._call("DELETE", f"/sandboxes/{_segment(id)}")

    def pause_sandbox(self, id: str) -> dict:
        return self._call("POST", f"/v1/sandboxes/{_segment(id)}/pause")

    def resume_sandbox(self, id: str) -> dict:
        return self._call("POST", f"/v1/sandboxes/{_segment(id)}/resume")

    def connect_sandbox(self, id: str) -> dict:
        return self._call("POST", f"/sandboxes/{_segment(id)}/connect")

    def set_sandbox_timeout(self, id: str) -> None:
        self._call("POST", f"/sandboxes/{_segment(id)}/timeout")

    def poweroff_sandbox(self, id: str) -> dict:
        return self._call("POST", f"/v1/sandboxes/{_segment(id)}/poweroff")

    def poweron_sandbox(self, id: str) -> dict:
        return self._call("POST", f"/v1/sandboxes/{_segment(id)}/poweron")

    def reboot_sandbox(self, id: str) -> dict:
        return self._call("POST", f"/v1/sandboxes/{_segment(id)}/reboot")

    def get_balloon(self, id: str) -> dict:
        return self._call("GET", f"/v1/sandboxes/{_segment(id)}/balloon")

    def update_balloon(self, id: str, amount_mib: int) -> dict:
        return self._call("PATCH", f"/v1/sandboxes/{_segment(id)}/balloon", {"amountMiB": amount_mib})

    def get_balloon_stats(self, id: str) -> dict:
        return self._call("GET", f"/v1/sandboxes/{_segment(id)}/balloon/statistics")

    def update_balloon_stats(self, id: str, stats_polling_interval_s: int) -> dict:
        return self._call(
            "PATCH",
            f"/v1/sandboxes/{_segment(id)}/balloon/statistics",
            {"statsPollingIntervalS": stats_polling_interval_s},
        )

    def get_balloon_hinting(self, id: str) -> dict:
        return self._call("GET", f"/v1/sandboxes/{_segment(id)}/balloon/hinting")

    def start_balloon_hinting(self, id: str, acknowledge_on_stop: bool = True) -> dict:
        return self._call(
            "POST",
            f"/v1/sandboxes/{_segment(id)}/balloon/hinting/start",
            {"acknowledgeOnStop": acknowledge_on_stop},
        )

    def stop_balloon_hinting(self, id: str) -> dict:
        return self._call("POST", f"/v1/sandboxes/{_segment(id)}/balloon/hinting/stop")

    def list_templates(self) -> list:
        return self._call("GET", "/templates")

    def get_template(self, id: str) -> dict:
        return self._call("GET", f"/templates/{_segment(id)}")

    def delete_template(self, id: str) -> None:
        self._call("DELETE", f"/templates/{_segment(id)}")

    def get_template_build_status(self, template_id: str, build_id: str) -> dict:
        return self._call("GET", f"/templates/{_segment(template_id)}/builds/{_segment(build_id)}/status")

    def start_template_build_v2(self, template_id: str, build_id: str, req: dict) -> None:
        self._call("POST", f"/v2/templates/{_segment(template_id)}/builds/{_segment(build_id)}", req)

    def create_template_v3(self, req: dict) -> dict:
        return self._call("POST", "/v3/templates", req)

    def convert_template(self, req: dict) -> dict:
        return self._call("POST", "/v1/templates/convert", req)

    def create_image(self, req: dict) -> dict:
        template_id = req.get("templateID") if isinstance(req, Mapping) else None
        if not isinstance(template_id, str) or not template_id:
            raise BoxApiError("templateID is required", 400, "invalid_argument")
        return self._call("POST", "/v1/images", req)

    def list_images(self) -> dict:
        return self._call("GET", "/v1/images")

    def get_image(self, id: str) -> dict:
        return self._call("GET", f"/v1/images/{_segment(id)}")

    def delete_image(self, id: str) -> None:
        self._call("DELETE", f"/v1/images/{_segment(id)}")

    def list_runtimes(self) -> dict:
        return self._call("GET", "/v1/runtimes")

    def get_runtime(self, type: str) -> dict:
        return self._call("GET", f"/v1/runtimes/{_segment(type)}")

    def get_runtime_capabilities(self, type: str) -> dict:
        return self._call("GET", f"/v1/runtimes/{_segment(type)}/capabilities")

    def list_strategies(self) -> dict:
        return self._call("GET", "/v1/scheduler/strategies")

    def set_strategy(self, req: dict) -> dict:
        return self._call("POST", "/v1/scheduler/strategies", req)

    def get_strategy(self, id: str) -> dict:
        return self._call("GET", f"/v1/scheduler/strategies/{_segment(id)}")

    def delete_strategy(self, id: str) -> dict:
        return self._call("DELETE", f"/v1/scheduler/strategies/{_segment(id)}")

    def get_default_strategy(self) -> dict:
        return self._call("GET", "/v1/scheduler/strategies/default")

    def set_default_strategy(self, id: str) -> dict:
        return self._call("PUT", "/v1/scheduler/strategies/default", {"id": id})

    def get_quotas(self) -> dict:
        return self._call("GET", "/v1/scheduler/quotas")

    def update_quota(self, req: dict) -> dict:
        return self._call("PUT", "/v1/scheduler/quotas", req)

    def decide(self, req: dict) -> dict:
        return self._call("POST", "/v1/scheduler/decide", _pascal_decide_request(req))


__all__ = ["BoxApiClient"]
