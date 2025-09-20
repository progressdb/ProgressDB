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
        """Make an HTTP request to the ProgressDB API.

        Parameters:
            method: HTTP method (GET/POST/...)
            path: request path beginning with '/'
            body: optional JSON-serializable request body
            headers: optional additional headers

        Returns:
            parsed JSON response or raw text; raises ApiError on non-2xx.
        """
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
        """Request server to create an HMAC signature for `user_id`.

        Returns a dict {"userId": ..., "signature": ...}.
        """
        return self.request("POST", "/v1/_sign", {"userId": user_id})

    def admin_health(self) -> Dict[str, Any]:
        """Admin health endpoint.

        Returns a JSON object describing service health.
        """
        return self.request("GET", "/admin/health")

    def admin_stats(self) -> Dict[str, Any]:
        """Admin stats endpoint.

        Returns aggregated statistics such as thread/message counts.
        """
        return self.request("GET", "/admin/stats")

    # Threads
    def list_threads(self, author: str, title: Optional[str] = None, slug: Optional[str] = None) -> Dict[str, Any]:
        """List threads.

        Parameters:
            author: required backend author id; sent as `X-User-ID`.
            title: optional substring filter on title.
            slug: optional exact slug filter.

        Raises:
            ValueError: if `author` is empty.
        """
        if not author:
            raise ValueError("author is required for backend list_threads calls")
        qs = [f"author={author}"]
        if title is not None:
            qs.append(f"title={title}")
        if slug is not None:
            qs.append(f"slug={slug}")
        path = "/v1/threads" + ("?" + "&".join(qs) if qs else "")
        return self.request("GET", path, headers={"X-User-ID": author})

    def create_thread(self, thread: Dict[str, Any], author: str) -> Dict[str, Any]:
        """Create a thread.

        Parameters:
            thread: thread payload (title, metadata, ...)
            author: required backend author id; sent as `X-User-ID`.

        Raises:
            ValueError: if `author` is empty.
        """
        if not author:
            raise ValueError("author is required for backend create_thread calls")
        return self.request("POST", "/v1/threads", thread, headers={"X-User-ID": author})

    def update_thread(self, id: str, thread: Dict[str, Any], author: str) -> Dict[str, Any]:
        """Update thread metadata.

        Parameters:
            id: thread id
            thread: partial thread payload
            author: required backend author id; sent as `X-User-ID`.

        Raises:
            ValueError: if `author` is empty.
        """
        if not author:
            raise ValueError("author is required for backend update_thread calls")
        return self.request("PUT", f"/v1/threads/{id}", thread, headers={"X-User-ID": author})

    def get_thread(self, id: str, author: str) -> Dict[str, Any]:
        """Retrieve thread metadata by id.

        Parameters:
            id: thread id
            author: required backend author id; sent as `X-User-ID`.

        Raises:
            ValueError: if `author` is empty.
        """
        if not author:
            raise ValueError("author is required for backend get_thread calls")
        path = f"/v1/threads/{id}"
        return self.request("GET", path, headers={"X-User-ID": author})

    def delete_thread(self, id: str, author: str):
        """Delete a thread (soft-delete).

        Parameters:
            id: thread id
            author: required backend author id; sent as `X-User-ID`.

        Raises:
            ValueError: if `author` is empty.
        """
        if not author:
            raise ValueError("author is required for backend delete_thread calls")
        return self.request("DELETE", f"/v1/threads/{id}", headers={"X-User-ID": author})

    # Messages
    def list_messages(self, thread: Optional[str] = None, limit: Optional[int] = None) -> Dict[str, Any]:
        """List messages across threads or within a specific thread.

        Parameters:
            thread: optional thread id to scope listing
            limit: optional maximum number of most recent messages
        """
        qs = []
        if thread is not None:
            qs.append(f"thread={thread}")
        if limit is not None:
            qs.append(f"limit={limit}")
        path = "/v1/messages" + ("?" + "&".join(qs) if qs else "")
        return self.request("GET", path)

    def create_message(self, msg: Dict[str, Any], author: str) -> Dict[str, Any]:
        """Create a message.

        Parameters:
            msg: message payload
            author: required backend author id; sent as `X-User-ID`.

        Raises:
            ValueError: if `author` is empty.
        """
        if not author:
            raise ValueError("author is required for backend create_message calls")
        return self.request("POST", "/v1/messages", msg, headers={"X-User-ID": author})

    def list_thread_messages(self, thread_id: str, limit: Optional[int] = None, author: Optional[str] = None) -> Dict[str, Any]:
        """List messages in a thread.

        Parameters:
            thread_id: thread id
            limit: optional maximum number of most recent messages to return
            author: optional backend author id to send as `X-User-ID`

        Returns:
            server response parsed as JSON (dict)
        """
        qs = []
        if limit is not None:
            qs.append(f"limit={limit}")
        path = f"/v1/threads/{thread_id}/messages" + ("?" + "&".join(qs) if qs else "")
        headers = {"X-User-ID": author} if author else None
        return self.request("GET", path, headers=headers)

    def get_thread_message(self, thread_id: str, id: str, author: Optional[str] = None) -> Dict[str, Any]:
        """Get a single message by id within a thread.

        Parameters:
            thread_id: thread id
            id: message id
            author: optional backend author id
        """
        headers = {"X-User-ID": author} if author else None
        return self.request("GET", f"/v1/threads/{thread_id}/messages/{id}", headers=headers)

    def update_thread_message(self, thread_id: str, id: str, msg: Dict[str, Any], author: Optional[str] = None) -> Dict[str, Any]:
        """Update (append new version) a message within a thread.

        Parameters:
            thread_id: thread id
            id: message id
            msg: updated message payload
            author: optional backend author id
        """
        headers = {"X-User-ID": author} if author else None
        return self.request("PUT", f"/v1/threads/{thread_id}/messages/{id}", body=msg, headers=headers)

    def delete_thread_message(self, thread_id: str, id: str, author: Optional[str] = None):
        """Soft-delete (append tombstone) a message within a thread.

        Parameters:
            thread_id: thread id
            id: message id
            author: optional backend author id
        """
        headers = {"X-User-ID": author} if author else None
        return self.request("DELETE", f"/v1/threads/{thread_id}/messages/{id}", headers=headers)

    def list_message_versions(self, thread_id: str, id: str, author: Optional[str] = None) -> Dict[str, Any]:
        """List versions for a message id under a thread (thread-scoped).

        Parameters:
            thread_id: thread id
            id: message id
            author: optional backend author id (X-User-ID)
        """
        headers = {"X-User-ID": author} if author else None
        return self.request("GET", f"/v1/threads/{thread_id}/messages/{id}/versions", headers=headers)

    def list_reactions(self, thread_id: str, id: str, author: Optional[str] = None) -> Dict[str, Any]:
        headers = {"X-User-ID": author} if author else None
        return self.request("GET", f"/v1/threads/{thread_id}/messages/{id}/reactions", headers=headers)

    def add_or_update_reaction(self, thread_id: str, id: str, input: Dict[str, Any], author: Optional[str] = None) -> Dict[str, Any]:
        headers = {"X-User-ID": author} if author else None
        return self.request("POST", f"/v1/threads/{thread_id}/messages/{id}/reactions", body=input, headers=headers)

    def remove_reaction(self, thread_id: str, id: str, identity: str, author: Optional[str] = None):
        headers = {"X-User-ID": author} if author else None
        return self.request("DELETE", f"/v1/threads/{thread_id}/messages/{id}/reactions/{identity}", headers=headers)

    # message-level get/update/delete removed; use thread-scoped APIs
