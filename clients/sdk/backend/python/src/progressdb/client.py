"""
ProgressDB Python SDK - Backend Client

Backend-only client for ProgressDB API.
Uses backend API keys to call frontend routes without signature requirements.
"""

from typing import Optional
from urllib.parse import urlencode

from .http import HTTPClient
from .types import (
    SDKOptionsType,
    ThreadCreateRequestType,
    ThreadUpdateRequestType,
    MessageCreateRequestType,
    MessageUpdateRequestType,
    ThreadListQueryType,
    MessageListQueryType,
    CreateThreadResponseType,
    UpdateThreadResponseType,
    DeleteThreadResponseType,
    CreateMessageResponseType,
    UpdateMessageResponseType,
    DeleteMessageResponseType,
    ThreadResponseType,
    MessageResponseType,
    ThreadsListResponseType,
    MessagesListResponseType,
    HealthzResponseType,
    ReadyzResponseType,
)


class ProgressDBClient:
    """Backend-only ProgressDB client.
    
    Uses backend API keys to call frontend routes without signature requirements.
    All methods are synchronous for ease of use.
    """
    
    def __init__(self, opts: SDKOptionsType):
        """Initialize ProgressDB client.
        
        Args:
            opts: Client configuration including baseUrl and apiKey
        """
        self.base_url = opts.get('baseUrl', '').rstrip('/')
        self.api_key = opts.get('apiKey')
        self.http = HTTPClient(opts)

    # Health endpoints
    def healthz(self) -> HealthzResponseType:
        """Basic health check."""
        return self.http.request("/healthz")

    def readyz(self) -> ReadyzResponseType:
        """Readiness check with version."""
        return self.http.request("/readyz")

    # Thread operations
    def create_thread(self, thread: ThreadCreateRequestType, user_id: str) -> CreateThreadResponseType:
        """Create a new thread."""
        return self.http.request("/frontend/v1/threads", "POST", thread, user_id)

    def list_threads(self, query: Optional[ThreadListQueryType] = None, user_id: Optional[str] = None) -> ThreadsListResponseType:
        """List threads with optional query parameters."""
        path = "/frontend/v1/threads"
        if query:
            path += f"?{urlencode(query)}"
        return self.http.request(path, "GET", None, user_id)

    def get_thread(self, thread_key: str, user_id: str) -> ThreadResponseType:
        """Get a specific thread."""
        encoded_key = HTTPClient.encode_url_component(thread_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_key}", "GET", None, user_id)

    def update_thread(self, thread_key: str, thread: ThreadUpdateRequestType, user_id: str) -> UpdateThreadResponseType:
        """Update a thread."""
        encoded_key = HTTPClient.encode_url_component(thread_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_key}", "PUT", thread, user_id)

    def delete_thread(self, thread_key: str, user_id: str) -> DeleteThreadResponseType:
        """Delete a thread."""
        encoded_key = HTTPClient.encode_url_component(thread_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_key}", "DELETE", None, user_id)

    # Message operations
    def create_thread_message(self, thread_key: str, message: MessageCreateRequestType, user_id: str) -> CreateMessageResponseType:
        """Create a message in a thread."""
        encoded_key = HTTPClient.encode_url_component(thread_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_key}/messages", "POST", message, user_id)

    def list_thread_messages(self, thread_key: str, query: Optional[MessageListQueryType] = None, user_id: Optional[str] = None) -> MessagesListResponseType:
        """List messages in a thread with optional query parameters."""
        encoded_key = HTTPClient.encode_url_component(thread_key)
        path = f"/frontend/v1/threads/{encoded_key}/messages"
        if query:
            path += f"?{urlencode(query)}"
        return self.http.request(path, "GET", None, user_id)

    def get_thread_message(self, thread_key: str, message_key: str, user_id: str) -> MessageResponseType:
        """Get a specific message in a thread."""
        encoded_thread_key = HTTPClient.encode_url_component(thread_key)
        encoded_message_key = HTTPClient.encode_url_component(message_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_thread_key}/messages/{encoded_message_key}", "GET", None, user_id)

    def update_thread_message(self, thread_key: str, message_key: str, message: MessageUpdateRequestType, user_id: str) -> UpdateMessageResponseType:
        """Update a message in a thread."""
        encoded_thread_key = HTTPClient.encode_url_component(thread_key)
        encoded_message_key = HTTPClient.encode_url_component(message_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_thread_key}/messages/{encoded_message_key}", "PUT", message, user_id)

    def delete_thread_message(self, thread_key: str, message_key: str, user_id: str) -> DeleteMessageResponseType:
        """Delete a message in a thread."""
        encoded_thread_key = HTTPClient.encode_url_component(thread_key)
        encoded_message_key = HTTPClient.encode_url_component(message_key)
        return self.http.request(f"/frontend/v1/threads/{encoded_thread_key}/messages/{encoded_message_key}", "DELETE", None, user_id)

    # Signature operations
    def sign_user(self, user_id: str) -> dict:
        """Generate signature for user using backend signing endpoint.
        
        Args:
            user_id: User ID to generate signature for
            
        Returns:
            Dictionary containing userId and signature
        """
        return self.http.request("/backend/v1/sign", "POST", {"userId": user_id})