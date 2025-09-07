"""ProgressDB backend Python client

Lightweight HTTP client using `requests`.
"""
from typing import Any, Dict, Optional
import json
import time

import requests


class ApiError(Exception):
    def __init__(self, status: int, body: Any):
        super().__init__(f"API error {status}: {body}")
        self.status = status
        self.body = body


class ProgressDBClient:
    def __init__(self, base_url: str = "", api_key: Optional[str] = None, timeout: int = 10):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout

    def _headers(self, extra: Optional[Dict[str, str]] = None) -> Dict[str, str]:
        h = {"Content-Type": "application/json"}
        if self.api_key:
            h["X-API-Key"] = self.api_key
        if extra:
            h.update(extra)
        return h

    def request(self, method: str, path: str, body: Optional[Any] = None, headers: Optional[Dict[str, str]] = None):
        url = f"{self.base_url}{path}"
        h = self._headers(headers)
        data = None
        if body is not None:
            data = json.dumps(body)
        resp = requests.request(method, url, headers=h, data=data, timeout=self.timeout)
        if resp.status_code >= 400:
            try:
                content = resp.json()
            except Exception:
                content = resp.text
            raise ApiError(resp.status_code, content)
        if resp.status_code == 204:
            return None
        try:
            return resp.json()
        except Exception:
            return resp.text

    # Admin / backend methods
    def sign_user(self, user_id: str) -> Dict[str, str]:
        return self.request("POST", "/v1/_sign", {"userId": user_id})

    def admin_health(self) -> Dict[str, Any]:
        return self.request("GET", "/admin/health")

    def admin_stats(self) -> Dict[str, Any]:
        return self.request("GET", "/admin/stats")

    # Threads
    def list_threads(self) -> Dict[str, Any]:
        return self.request("GET", "/v1/threads")

    def create_thread(self, thread: Dict[str, Any]) -> Dict[str, Any]:
        return self.request("POST", "/v1/threads", thread)

    def get_thread(self, id: str) -> Dict[str, Any]:
        return self.request("GET", f"/v1/threads/{id}")

    def delete_thread(self, id: str):
        return self.request("DELETE", f"/v1/threads/{id}")

    # Messages
    def list_messages(self, thread: Optional[str] = None, limit: Optional[int] = None) -> Dict[str, Any]:
        qs = []
        if thread is not None:
            qs.append(f"thread={thread}")
        if limit is not None:
            qs.append(f"limit={limit}")
        path = "/v1/messages" + ("?" + "&".join(qs) if qs else "")
        return self.request("GET", path)

    def create_message(self, msg: Dict[str, Any]) -> Dict[str, Any]:
        return self.request("POST", "/v1/messages", msg)

    def get_message(self, id: str) -> Dict[str, Any]:
        return self.request("GET", f"/v1/messages/{id}")

    def update_message(self, id: str, msg: Dict[str, Any]) -> Dict[str, Any]:
        return self.request("PUT", f"/v1/messages/{id}", msg)

    def delete_message(self, id: str):
        return self.request("DELETE", f"/v1/messages/{id}")
