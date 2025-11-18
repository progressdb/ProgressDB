from typing import TypedDict, Optional


class ThreadType(TypedDict):
    key: str
    title: Optional[str]
    created_ts: Optional[int]
    updated_ts: Optional[int]
    author: str
    deleted: Optional[bool]
    kms: Optional[dict]  # KMSMetaType


class ThreadCreateRequestType(TypedDict):
    title: str


class ThreadUpdateRequestType(TypedDict, total=False):
    title: str