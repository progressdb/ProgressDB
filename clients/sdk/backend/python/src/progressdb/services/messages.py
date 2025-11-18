"""Messages service for ProgressDB Python SDK."""

from typing import TYPE_CHECKING, Optional
from urllib.parse import urlencode

from ..types.queries import MessageListQueryType
from ..types.responses import (
    CreateMessageResponseType,
    DeleteMessageResponseType,
    MessageResponseType,
    MessagesListResponseType,
    UpdateMessageResponseType,
)
from ..types.message import MessageCreateRequestType, MessageUpdateRequestType
from ..utils.validation import validate_pagination_query

if TYPE_CHECKING:
    from ..client.http import HTTPClient


class MessagesService:
    """Service for message operations."""

    def __init__(self, http_client: "HTTPClient") -> None:
        """Initialize messages service.
        
        Args:
            http_client: HTTP client instance
        """
        self.http_client = http_client

    async def list_thread_messages(
        self,
        thread_key: str,
        query: MessageListQueryType = {},
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> MessagesListResponseType:
        """List messages for a thread.
        
        Args:
            thread_key: Thread key
            query: Optional query parameters (limit, before, after, anchor, sort_by)
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            List of messages
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
        encoded_thread_key = self._encode_url_component(thread_key)
        url = f"/frontend/v1/threads/{encoded_thread_key}/messages{f'?{query_string}' if query_string else ''}"
        
        return await self.http_client.request(url, "GET", None, user_id, user_signature)

    async def create_thread_message(
        self,
        thread_key: str,
        msg: MessageCreateRequestType,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> CreateMessageResponseType:
        """Create a message within a thread.
        
        Args:
            thread_key: Thread key
            msg: Message payload
            user_id: Optional user id to send as X-User-ID
            user_signature: Optional signature to send as X-User-Signature
            
        Returns:
            Created message response
        """
        encoded_thread_key = self._encode_url_component(thread_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_thread_key}/messages",
            "POST",
            msg,
            user_id,
            user_signature,
        )

    async def get_thread_message(
        self,
        thread_key: str,
        message_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> MessageResponseType:
        """Retrieve a message by key within a thread.
        
        Args:
            thread_key: Thread key
            message_key: Message key
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            Message data
        """
        encoded_thread_key = self._encode_url_component(thread_key)
        encoded_message_key = self._encode_url_component(message_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_thread_key}/messages/{encoded_message_key}",
            "GET",
            None,
            user_id,
            user_signature,
        )

    async def update_thread_message(
        self,
        thread_key: str,
        message_key: str,
        msg: MessageUpdateRequestType,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> UpdateMessageResponseType:
        """Update a message within a thread.
        
        Args:
            thread_key: Thread key
            message_key: Message key
            msg: Message payload
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            Updated message response
        """
        encoded_thread_key = self._encode_url_component(thread_key)
        encoded_message_key = self._encode_url_component(message_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_thread_key}/messages/{encoded_message_key}",
            "PUT",
            msg,
            user_id,
            user_signature,
        )

    async def delete_thread_message(
        self,
        thread_key: str,
        message_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> DeleteMessageResponseType:
        """Soft-delete a message within a thread.
        
        Args:
            thread_key: Thread key
            message_key: Message key
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            Delete response
        """
        encoded_thread_key = self._encode_url_component(thread_key)
        encoded_message_key = self._encode_url_component(message_key)
        return await self.http_client.request(
            f"/frontend/v1/threads/{encoded_thread_key}/messages/{encoded_message_key}",
            "DELETE",
            None,
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