from __future__ import annotations


class BoxApiError(Exception):
    def __init__(self, message: str, status: int = 0, code: str = "http_error") -> None:
        super().__init__(message)
        self.status = status
        self.code = code


class BoxdError(Exception):
    def __init__(self, message: str, status: int = 0) -> None:
        super().__init__(message)
        self.status = status
