from typing import TypedDict, Any, List, Optional


class KeyResponseType(TypedDict):
    key: str

# Create/Update/Delete response types
CreateThreadResponseType = KeyResponseType
CreateMessageResponseType = KeyResponseType
UpdateThreadResponseType = KeyResponseType
UpdateMessageResponseType = KeyResponseType
DeleteThreadResponseType = KeyResponseType
DeleteMessageResponseType = KeyResponseType

# Standardized response wrapper pattern
class ApiResponseType(TypedDict):
    data: Any

class ThreadApiResponseType(TypedDict):
    data: Any

class MessageApiResponseType(TypedDict):
    data: Any

class ThreadsListApiResponseType(TypedDict):
    data: List[Any]

class MessagesListApiResponseType(TypedDict):
    data: List[Any]

# Response types used by services
class ThreadResponseType(TypedDict):
    thread: Any

class MessageResponseType(TypedDict):
    message: Any

class ThreadsListResponseType(TypedDict):
    threads: List[Any]
    pagination: Any

class MessagesListResponseType(TypedDict):
    thread: str
    messages: List[Any]
    pagination: Any

# Health check response types
class HealthzResponseType(TypedDict):
    status: str

class ReadyzResponseType(TypedDict):
    status: str
    version: Optional[str]