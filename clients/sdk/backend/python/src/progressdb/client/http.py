import json
import time
from typing import Optional, Dict, Any, Union
from urllib.parse import urlencode

import requests


class HTTPClient:
    def __init__(self, opts: Dict[str, Any]):
        self.base_url = opts.get('baseUrl', '').rstrip('/')
        self.api_key = opts.get('apiKey')
        self.default_user_id = opts.get('defaultUserId')
        self.default_user_signature = opts.get('defaultUserSignature')
        self.mode = opts.get('mode', 'frontend')  # 'frontend' | 'backend'
        self.timeout = opts.get('timeout', 10)
        
        # Backend signature caching
        self._signature_cache: Dict[str, Dict[str, Any]] = {}

    def _headers(
        self, 
        user_id: Optional[str] = None, 
        user_signature: Optional[str] = None, 
        has_body: bool = True
    ) -> Dict[str, str]:
        """Build headers for a request using provided or default user credentials."""
        if self.mode == 'backend':
            # Backend mode: use Authorization Bearer
            headers: Dict[str, str] = {}
            if self.api_key:
                headers['Authorization'] = f'Bearer {self.api_key}'
            uid = user_id or self.default_user_id
            if uid:
                headers['X-User-ID'] = uid
            sig = user_signature or self.default_user_signature
            if sig:
                headers['X-User-Signature'] = sig
            if has_body:
                headers['Content-Type'] = 'application/json'
            return headers
        else:
            # Frontend mode: use X-API-Key
            headers: Dict[str, str] = {}
            if self.api_key:
                headers['X-API-Key'] = self.api_key
            uid = user_id or self.default_user_id
            if uid:
                headers['X-User-ID'] = uid
            sig = user_signature or self.default_user_signature
            if sig:
                headers['X-User-Signature'] = sig
            if has_body:
                headers['Content-Type'] = 'application/json'
            return headers

    async def request(
        self, 
        path: str, 
        method: str = 'GET', 
        body: Optional[Any] = None, 
        user_id: Optional[str] = None, 
        user_signature: Optional[str] = None
    ) -> Union[Dict[str, Any], str, None]:
        """
        Internal helper to perform a fetch request against the configured base URL.
        Returns parsed JSON when Content-Type is application/json, otherwise returns text.
        
        Args:
            path: request path
            method: HTTP method
            body: optional request body
            user_id: optional user id to attach as X-User-ID
            user_signature: optional signature to attach as X-User-Signature
        """
        url = f"{self.base_url}{path}"
        has_body = body is not None and method not in ['GET', 'DELETE']
        headers = self._headers(user_id, user_signature, has_body)
        
        try:
            response = requests.request(
                method,
                url,
                headers=headers,
                json=body if has_body else None,
                timeout=self.timeout
            )
            
            if response.status_code == 204:
                return None
            
            # Handle error responses
            if not response.ok:
                content_type = response.headers.get('content-type', '')
                if 'application/json' in content_type:
                    error_data = response.json()
                    raise Exception(error_data.get('error', 'API request failed'))
                raise Exception(f'HTTP {response.status_code}: {response.text}')
            
            # Parse successful responses
            content_type = response.headers.get('content-type', '')
            if 'application/json' in content_type:
                return response.json()
            return response.text
            
        except requests.RequestException as e:
            raise Exception(f'Request failed: {str(e)}')

    async def _get_or_generate_signature(self, user_id: str) -> str:
        """Get cached signature or generate new one (backend mode only)."""
        if self.mode != 'backend':
            return self.default_user_signature or ''
            
        # Check cache
        cached = self._signature_cache.get(user_id)
        now = int(time.time() * 1000)  # milliseconds
        
        if cached and cached['expires'] > now:
            return cached['signature']
        
        # Generate new signature
        sign_url = f"{self.base_url}/backend/v1/sign"
        headers = {'Authorization': f'Bearer {self.api_key}', 'Content-Type': 'application/json'}
        
        response = requests.post(sign_url, headers=headers, json={'userId': user_id}, timeout=self.timeout)
        if not response.ok:
            raise Exception(f'Failed to generate signature: {response.text}')
            
        data = response.json()
        signature = data['signature']
        
        # Cache for 5 minutes
        self._signature_cache[user_id] = {
            'signature': signature,
            'expires': now + (5 * 60 * 1000)  # 5 minutes TTL
        }
        
        return signature

    def clear_signature_cache(self, user_id: Optional[str] = None) -> None:
        """Clear signature cache for specific user or all users."""
        if user_id:
            self._signature_cache.pop(user_id, None)
        else:
            self._signature_cache.clear()

    def get_cache_stats(self) -> Dict[str, Any]:
        """Get signature cache statistics."""
        return {
            'size': len(self._signature_cache),
            'entries': dict(self._signature_cache)
        }