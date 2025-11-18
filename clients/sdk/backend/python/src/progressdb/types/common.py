from typing import Optional, TypedDict


class SDKOptionsType(TypedDict, total=False):
    baseUrl: str
    apiKey: str  # frontend API key sent as X-API-Key
    defaultUserId: str
    defaultUserSignature: str
    mode: str  # "frontend" | "backend"
    timeout: int