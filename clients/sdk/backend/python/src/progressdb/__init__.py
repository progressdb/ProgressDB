"""
ProgressDB Python SDK

Backend-only client for ProgressDB API.
Uses backend API keys to call frontend routes without signature requirements.
"""

from typing import Optional

# Export main client
from .client import ProgressDBClient

# Export all types
from .types import (
    # Configuration types
    SDKOptionsType,
    
    # Core types
    MessageType,
    ThreadType,
    
    # Request types
    ThreadCreateRequestType,
    ThreadUpdateRequestType,
    MessageCreateRequestType,
    MessageUpdateRequestType,
    
    # Query types
    ThreadListQueryType,
    MessageListQueryType,
    
    # Response types
    PaginationResponseType,
    ThreadResponseType,
    MessageResponseType,
    ThreadsListResponseType,
    MessagesListResponseType,
    CreateThreadResponseType,
    UpdateThreadResponseType,
    DeleteThreadResponseType,
    CreateMessageResponseType,
    UpdateMessageResponseType,
    DeleteMessageResponseType,
    HealthzResponseType,
    ReadyzResponseType,
    
    # Error types
    ApiErrorResponseType,
)

# Export error classes
from .errors import (
    ProgressDBError,
    ApiError,
    NetworkError,
    ValidationError,
)


# Factory function matching Node.js SDK pattern
def ProgressDB(options: SDKOptionsType) -> ProgressDBClient:
    """ProgressDB factory - returns a ready-to-use ProgressDBClient instance.
    
    Args:
        options: Client configuration including baseUrl and apiKey
        
    Returns:
        Configured ProgressDBClient instance
    """
    return ProgressDBClient(options)


# Export main client class for direct instantiation
__all__ = [
    # Main client and factory
    "ProgressDBClient",
    "ProgressDB",
    
    # Configuration types
    "SDKOptionsType",
    
    # Core types
    "MessageType",
    "ThreadType",
    
    # Request types
    "ThreadCreateRequestType",
    "ThreadUpdateRequestType",
    "MessageCreateRequestType",
    "MessageUpdateRequestType",
    
    # Query types
    "ThreadListQueryType",
    "MessageListQueryType",
    
    # Response types
    "PaginationResponseType",
    "ThreadResponseType",
    "MessageResponseType",
    "ThreadsListResponseType",
    "MessagesListResponseType",
    "CreateThreadResponseType",
    "UpdateThreadResponseType",
    "DeleteThreadResponseType",
    "CreateMessageResponseType",
    "UpdateMessageResponseType",
    "DeleteMessageResponseType",
    "HealthzResponseType",
    "ReadyzResponseType",
    
    # Error types
    "ApiErrorResponseType",
    
    # Error classes
    "ProgressDBError",
    "ApiError",
    "NetworkError",
    "ValidationError",
]