from __future__ import annotations

from typing import Any, Dict, List, Optional

from .auth import resolve_data_url
from .boxapi import BoxApiClient
from .boxd import BoxdClient


class Gpurun:
    def __init__(
        self,
        api_key: Optional[str] = None,
        access_token: Optional[str] = None,
        base_url: Optional[str] = None,
        data_url: Optional[str] = None,
        token: Optional[str] = None,
        sandbox_id: Optional[str] = None,
        timeout: Optional[float] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> None:
        self.sandbox_id = sandbox_id
        self.box = BoxApiClient(
            api_key=api_key,
            access_token=access_token,
            base_url=base_url,
            token=token,
            timeout=timeout,
            headers=headers,
        )
        self.data = BoxdClient(
            api_key=api_key,
            access_token=access_token,
            base_url=resolve_data_url(data_url),
            token=token,
            sandbox_id=sandbox_id,
            timeout=timeout,
            headers=headers,
        )

    def create_sandbox(self, req: dict) -> dict:
        return self.box.create_sandbox(req)

    def exec(self, cmd: List[str], env_vars: Optional[Dict[str, str]] = None, sandbox_id: Optional[str] = None) -> dict:
        return self.data.exec(cmd, env_vars, sandbox_id)


__all__ = ["Gpurun"]
