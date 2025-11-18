"""Threads service for ProgressDB Python SDK."""

from typing import TYPE_CHECKING, Optional
from urllib.parse import urlencode

from ..types.queries import ThreadListQueryType
from ..types.responses import (
    CreateThreadResponseType,
    DeleteThreadResponseType,
    ThreadResponseType,
    ThreadsListResponseType,
    UpdateThreadResponseType,
)
from ..types.thread import ThreadCreateRequestType, ThreadUpdateRequestType
from ..utils.validation import validate_pagination_query

if TYPE_CHECKING:
    from ..client.http import HTTPClient


class ThreadsService:
    """Service for thread operations."""

    def __init__(self, http_client: "HTTPClient") -> None:
        """Initialize threads service.
        
        Args:
            http_client: HTTP client instance
        """
        self.http_client = http_client

    async def create_thread(
        self,
        thread: ThreadCreateRequestType,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> CreateThreadResponseType:
        """Create a new thread.
        
        Args:
            thread: Thread payload with required title
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Created thread response
        """
        return await self.http_client.request(
            "/frontend/v1/threads",
            "POST",
            thread,
            user_id,
            user_signature,
        )

    async def list_threads(
        self,
        query: ThreadListQueryType = {},
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> ThreadsListResponseType:
        """List threads visible to the current user.
        
        Args:
            query: Optional query parameters (limit, before, after, anchor, sort_by)
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            List of threads
        """
        validate_pagination_query(query)
        
        qs_params = {}
        if query.get("limit") is not None:
            qs_params["limit"] = str(query["limit"])
        if query.get("before"):
            qs_params["before"] = query["before"]
        if query.get("after"):
            qs_params["after"] = query["after"]
        if query.get("anchor"):
            qs_params["anchor"] = query["anchor"]
        if query.get("sort_by"):
            qs_params["sort_by"] = query["sort_by"]
            
        query_string = urlencode(qs_params)
        url = f"/frontend/v1/threads{f'?{query_string}' if query_string else ''}"
        
        return await self.http_client.request(url, "GET", None, user_id, user_signature)

    async def get_thread(
        self,
        thread_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> ThreadResponseType:
        """Retrieve thread metadata by key.
        
        Args:
            thread_key: Thread key
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Thread metadata
        """
        encoded_key = self._encode_url_component(thread_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_key}",
            "GET",
            None,
            user_id,
            user_signature,
        )

    async def delete_thread(
        self,
        thread_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> DeleteThreadResponseType:
        """Soft-delete a thread by key.
        
        Args:
            thread_key: Thread key
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Delete response
        """
        encoded_key = self._encode_url_component(thread_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_key}",
            "DELETE",
            None,
            user_id,
            user_signature,
        )

    async def update_thread(
        self,
        thread_key: str,
        thread: ThreadUpdateRequestType,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> UpdateThreadResponseType:
        """Update thread metadata.
        
        Args:
            thread_key: Thread key
            thread: Partial thread payload (title)
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Updated thread response
        """
        encoded_key = self._encode_url_component(thread_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_key}",
            "PUT",
            thread,
            user_id,
            user_signature,
        )

    def _encode_url_component(self, component: str) -> str:
        """Encode URL component (similar to encodeURIComponent in JS).
        
        Args:
            component: String to encode
            
        Returns:
            URL-encoded string
        """
        # Python's quote is similar to encodeURIComponent
        from urllib.parse import quote
        return quote(component, safe='')