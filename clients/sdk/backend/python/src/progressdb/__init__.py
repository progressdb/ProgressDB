"""
ProgressDB Python SDK

A unified Python SDK that supports both frontend and backend modes.
- Frontend mode: Uses X-API-Key header for authentication
- Backend mode: Uses Authorization Bearer header with signature caching
- Async/await support throughout
- Type safety with TypedDict interfaces
"""

from typing import TYPE_CHECKING, Optional

# Export all types
from .types import (
    # Common types
    SDKOptionsType as SDKOptions,
    
    # Error types
    ApiErrorResponseType as ApiErrorResponse,
    
    # Message types
    MessageType,
    MessageCreateRequestType as MessageCreateRequest,
    MessageUpdateRequestType as MessageUpdateRequest,
    
    # Pagination types
    PaginationResponseType as PaginationResponse,
    
    # Query types
    ThreadListQueryType as ThreadListQuery,
    MessageListQueryType as MessageListQuery,
    
    # Response types
    HealthzResponseType as HealthzResponse,
    ReadyzResponseType as ReadyzResponse,
    ThreadResponseType as ThreadResponse,
    ThreadsListResponseType as ThreadsListResponse,
    CreateThreadResponseType as CreateThreadResponse,
    UpdateThreadResponseType as UpdateThreadResponse,
    DeleteThreadResponseType as DeleteThreadResponse,
    MessageResponseType as MessageResponse,
    MessagesListResponseType as MessagesListResponse,
    CreateMessageResponseType as CreateMessageResponse,
    UpdateMessageResponseType as UpdateMessageResponse,
    DeleteMessageResponseType as DeleteMessageResponse,
    
    # Thread types
    ThreadType,
    ThreadCreateRequestType as ThreadCreateRequest,
    ThreadUpdateRequestType as ThreadUpdateRequest,
)

# Import internal modules
from .client.http import HTTPClient
from .services.health import HealthService
from .services.messages import MessagesService
from .services.threads import ThreadsService


class ProgressDBClient:
    """ProgressDB Python SDK client supporting both frontend and backend modes."""

    def __init__(self, options: SDKOptions = None) -> None:
        """Create a new ProgressDBClient.
        
        Args:
            options: SDK options including mode, base_url, api_key, etc.
        """
        if options is None:
            options = {}
            
        self.http_client = HTTPClient(options)
        self.health_service = HealthService(self.http_client)
        self.messages_service = MessagesService(self.http_client)
        self.threads_service = ThreadsService(self.http_client)

        # Expose HTTP client properties for backward compatibility
        self.base_url = self.http_client.base_url
        self.api_key = self.http_client.api_key
        self.default_user_id = self.http_client.default_user_id
        self.default_user_signature = self.http_client.default_user_signature
        self.mode = self.http_client.mode

    # Health endpoints
    async def healthz(self) -> HealthzResponse:
        """Basic health check.
        
        Returns:
            Parsed JSON health object from GET /healthz
        """
        return await self.health_service.healthz()

    async def readyz(self) -> ReadyzResponse:
        """Readiness check with version info.
        
        Returns:
            Parsed JSON readiness object from GET /readyz
        """
        return await self.health_service.readyz()

    # Messages - thread-scoped only per OpenAPI spec
    async def list_thread_messages(
        self,
        thread_key: str,
        query: MessageListQuery = {},
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> MessagesListResponse:
        """List messages for a thread.
        
        Args:
            thread_key: Thread key
            query: Optional query parameters (limit, before, after, anchor, sort_by)
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            List of messages
        """
        return await self.messages_service.list_thread_messages(
            thread_key, query, user_id, user_signature
        )

    async def create_thread_message(
        self,
        thread_key: str,
        msg: MessageCreateRequest,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> CreateMessageResponse:
        """Create a message within a thread.
        
        Args:
            thread_key: Thread key
            msg: Message payload
            user_id: Optional user id to send as X-User-ID
            user_signature: Optional signature to send as X-User-Signature
            
        Returns:
            Created message response
        """
        return await self.messages_service.create_thread_message(
            thread_key, msg, user_id, user_signature
        )

    async def get_thread_message(
        self,
        thread_key: str,
        message_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> MessageResponse:
        """Retrieve a message by key within a thread.
        
        Args:
            thread_key: Thread key
            message_key: Message key
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            Message data
        """
        return await self.messages_service.get_thread_message(
            thread_key, message_key, user_id, user_signature
        )

    async def update_thread_message(
        self,
        thread_key: str,
        message_key: str,
        msg: MessageUpdateRequest,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> UpdateMessageResponse:
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
        return await self.messages_service.update_thread_message(
            thread_key, message_key, msg, user_id, user_signature
        )

    async def delete_thread_message(
        self,
        thread_key: str,
        message_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> DeleteMessageResponse:
        """Soft-delete a message within a thread.
        
        Args:
            thread_key: Thread key
            message_key: Message key
            user_id: Optional user id to attach as X-User-ID
            user_signature: Optional signature to attach as X-User-Signature
            
        Returns:
            Delete response
        """
        return await self.messages_service.delete_thread_message(
            thread_key, message_key, user_id, user_signature
        )

    # Threads
    async def create_thread(
        self,
        thread: ThreadCreateRequest,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> CreateThreadResponse:
        """Create a new thread.
        
        Args:
            thread: Thread payload with required title
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Created thread response
        """
        return await self.threads_service.create_thread(
            thread, user_id, user_signature
        )

    async def list_threads(
        self,
        query: ThreadListQuery = {},
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> ThreadsListResponse:
        """List threads visible to the current user.
        
        Args:
            query: Optional query parameters (limit, before, after, anchor, sort_by)
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            List of threads
        """
        return await self.threads_service.list_threads(
            query, user_id, user_signature
        )

    async def get_thread(
        self,
        thread_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> ThreadResponse:
        """Retrieve thread metadata by key.
        
        Args:
            thread_key: Thread key
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Thread metadata
        """
        return await self.threads_service.get_thread(
            thread_key, user_id, user_signature
        )

    async def delete_thread(
        self,
        thread_key: str,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> DeleteThreadResponse:
        """Soft-delete a thread by key.
        
        Args:
            thread_key: Thread key
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Delete response
        """
        return await self.threads_service.delete_thread(
            thread_key, user_id, user_signature
        )

    async def update_thread(
        self,
        thread_key: str,
        thread: ThreadUpdateRequest,
        user_id: Optional[str] = None,
        user_signature: Optional[str] = None,
    ) -> UpdateThreadResponse:
        """Update thread metadata.
        
        Args:
            thread_key: Thread key
            thread: Partial thread payload (title)
            user_id: Optional user id
            user_signature: Optional signature
            
        Returns:
            Updated thread response
        """
        return await self.threads_service.update_thread(
            thread_key, thread, user_id, user_signature
        )

    # Cache management methods (for backend mode)
    def clear_signature_cache(self) -> None:
        """Clear the signature cache (backend mode only)."""
        self.http_client.clear_signature_cache()

    def get_cache_stats(self) -> dict:
        """Get signature cache statistics (backend mode only).
        
        Returns:
            Dictionary with cache statistics
        """
        return self.http_client.get_cache_stats()


# Re-export for backward compatibility
__all__ = [
    # Main client
    "ProgressDBClient",
    
    # Types
    "SDKOptions",
    "ApiErrorResponse",
    "MessageType",
    "MessageCreateRequest",
    "MessageUpdateRequest",
    "PaginationResponse",
    "BeforeQuery",
    "AfterQuery",
    "AnchorQuery",
    "ThreadListQuery",
    "MessageListQuery",
    "HealthzResponse",
    "ReadyzResponse",
    "ThreadResponse",
    "ThreadsListResponse",
    "CreateThreadResponse",
    "UpdateThreadResponse",
    "DeleteThreadResponse",
    "MessageResponse",
    "MessagesListResponse",
    "CreateMessageResponse",
    "UpdateMessageResponse",
    "DeleteMessageResponse",
    "ThreadType",
    "ThreadCreateRequest",
    "ThreadUpdateRequest",
]