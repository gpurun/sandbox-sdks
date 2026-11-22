from __future__ import annotations

import os
from typing import Any, Optional


def _trimmed(value: Any) -> Optional[str]:
    if not isinstance(value, str):
        return None
    result = value.strip()
    return result or None


def _environment(primary: str, *fallbacks: str) -> Optional[str]:
    for key in (primary, *fallbacks):
        result = _trimmed(os.environ.get(key))
        if result is not None:
            return result
    return None


def resolve_api_key(api_key: Any = None) -> Optional[str]:
    return _trimmed(api_key) or _environment("GPURUN_API_KEY", "NOVITA_API_KEY")


def resolve_access_token(access_token: Any = None, token: Any = None) -> Optional[str]:
    return _trimmed(access_token) or _trimmed(token) or _environment("GPURUN_ACCESS_TOKEN", "NOVITA_ACCESS_TOKEN")


def resolve_bearer_token(api_key: Any = None, access_token: Any = None, token: Any = None) -> Optional[str]:
    return resolve_api_key(api_key) or resolve_access_token(access_token, token)


def resolve_base_url(base_url: Any = None) -> str:
    return _trimmed(base_url) or _environment("GPURUN_BOXAPI_URL") or "http://127.0.0.1:8080"


def resolve_data_url(data_url: Any = None) -> str:
    return _trimmed(data_url) or _environment("GPURUN_BOXD_URL") or "http://127.0.0.1:8082"
