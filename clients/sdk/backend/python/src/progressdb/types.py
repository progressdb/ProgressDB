"""
ProgressDB Python SDK Types

Consolidated type definitions for backend-only client.
"""

from typing import TypedDict, Any, NotRequired


# Core Types
class MessageType(TypedDict):
    """Message represents a single message stored in ProgressDB."""
    id: str
    thread: str
    author: str
    role: NotRequired[str]
    ts: int
    body: NotRequired[Any]
    reply_to: NotRequired[str]
    deleted: NotRequired[bool]
    reactions: NotRequired[dict[str, str]]


class ThreadType(TypedDict):
    """Thread metadata stored in ProgressDB."""
    id: str
    title: str
    author: str
    slug: NotRequired[str]
    created_ts: NotRequired[int]
    updated_ts: NotRequired[int]


# Request Types
class ThreadCreateRequestType(TypedDict):
    """Request to create a thread."""
    title: str


class ThreadUpdateRequestType(TypedDict, total=False):
    """Request to update a thread."""
    title: str


class MessageCreateRequestType(TypedDict):
    """Request to create a message."""
    body: Any  # Any JSON serializable data


class MessageUpdateRequestType(TypedDict):
    """Request to update a message."""
    body: Any  # Any JSON serializable data


# Query Types
class ThreadListQueryType(TypedDict, total=False):
    """Query parameters for listing threads."""
    limit: NotRequired[int]
    before: NotRequired[str]
    after: NotRequired[str]
    anchor: NotRequired[str]
    sort_by: NotRequired[str]


class MessageListQueryType(TypedDict, total=False):
    """Query parameters for listing messages."""
    limit: NotRequired[int]
    before: NotRequired[str]
    after: NotRequired[str]
    anchor: NotRequired[str]
    sort_by: NotRequired[str]


# Response Types
class PaginationResponseType(TypedDict):
    """Pagination metadata."""
    has_more: bool
    next_cursor: NotRequired[str]
    prev_cursor: NotRequired[str]


class ThreadResponseType(ThreadType):
    """Thread response with additional metadata."""
    # Inherits all fields from ThreadType
    pass


class MessageResponseType(MessageType):
    """Message response with additional metadata."""
    # Inherits all fields from MessageType
    pass


class ThreadsListResponseType(TypedDict):
    """Response for listing threads."""
    data: list[ThreadResponseType]
    pagination: PaginationResponseType


class MessagesListResponseType(TypedDict):
    """Response for listing messages."""
    data: list[MessageResponseType]
    pagination: PaginationResponseType


class CreateThreadResponseType(ThreadResponseType):
    """Response for creating a thread."""
    pass


class UpdateThreadResponseType(ThreadResponseType):
    """Response for updating a thread."""
    pass


class DeleteThreadResponseType(TypedDict):
    """Response for deleting a thread."""
    deleted: bool


class CreateMessageResponseType(MessageResponseType):
    """Response for creating a message."""
    pass


class UpdateMessageResponseType(MessageResponseType):
    """Response for updating a message."""
    pass


class DeleteMessageResponseType(TypedDict):
    """Response for deleting a message."""
    deleted: bool


class HealthzResponseType(TypedDict):
    """Health check response."""
    status: str


class ReadyzResponseType(TypedDict):
    """Readiness check response."""
    status: str
    version: NotRequired[str]


class ApiErrorResponseType(TypedDict):
    """API error response."""
    error: str
    message: NotRequired[str]


# Configuration Types
class SDKOptionsType(TypedDict, total=False):
    """Options for ProgressDBClient."""
    baseUrl: str
    apiKey: str
    timeout: NotRequired[int]
    fetch: NotRequired[Any]  # Custom fetch function