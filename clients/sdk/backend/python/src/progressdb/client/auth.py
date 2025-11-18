from typing import Optional, Dict


def build_headers(
    api_key: Optional[str] = None,
    user_id: Optional[str] = None,
    user_signature: Optional[str] = None,
    has_body: bool = True
) -> Dict[str, str]:
    """
    Build request headers used by frontend SDK.
    
    Args:
        api_key: frontend API key to send as X-API-Key
        user_id: optional user id to send as X-User-ID
        user_signature: optional signature to send as X-User-Signature
        has_body: whether the request has a body (affects Content-Type header)
    
    Returns:
        headers dictionary
    """
    headers: Dict[str, str] = {}
    if api_key:
        headers['X-API-Key'] = api_key
    if user_id:
        headers['X-User-ID'] = user_id
    if user_signature:
        headers['X-User-Signature'] = user_signature
    if has_body:
        headers['Content-Type'] = 'application/json'
    return headers