from typing import TypedDict, Any, Optional


class MessageType(TypedDict):
    key: str
    thread: str
    author: str
    created_ts: Optional[int]
    updated_ts: Optional[int]
    body: Optional[Any]
    deleted: Optional[bool]


class MessageCreateRequestType(TypedDict):
    body: Any  # Any JSON serializable data (object, string, number, etc.)


class MessageUpdateRequestType(TypedDict):
    body: Any  # Any JSON serializable data (object, string, number, etc.)