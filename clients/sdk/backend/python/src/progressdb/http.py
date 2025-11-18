"""
Simple HTTP client for ProgressDB Python SDK.

Backend-only: uses Authorization Bearer + X-User-ID headers.
No signature generation or caching needed.
"""

import json
from typing import Optional, Dict, Any, Union
from urllib.parse import quote

import requests
from .errors import ApiError


class HTTPClient:
    """Simple HTTP client for backend authentication."""
    
    def __init__(self, opts: Dict[str, Any]):
        """Initialize HTTP client with backend options.
        
        Args:
            opts: Configuration options including baseUrl, apiKey, timeout
        """
        self.base_url = opts.get('baseUrl', '').rstrip('/')
        self.api_key = opts.get('apiKey')
        self.timeout = opts.get('timeout', 10)
        self.fetch = opts.get('fetch')  # Custom fetch function (optional)

    def _headers(self, user_id: Optional[str] = None, has_body: bool = True) -> Dict[str, str]:
        """Build headers for backend authentication.
        
        Args:
            user_id: Optional user ID to send as X-User-ID
            has_body: Whether request has a body (for Content-Type)
            
        Returns:
            Dictionary of headers
        """
        headers: Dict[str, str] = {}
        
        # Backend authentication: Authorization Bearer
        if self.api_key:
            headers['Authorization'] = f'Bearer {self.api_key}'
            
        # User identification
        if user_id:
            headers['X-User-ID'] = user_id
            
        # Content-Type for requests with body
        if has_body:
            headers['Content-Type'] = 'application/json'
            
        return headers

    def request(
        self,
        path: str,
        method: str = 'GET',
        body: Optional[Any] = None,
        user_id: Optional[str] = None
    ) -> Union[Dict[str, Any], str, None]:
        """
        Make HTTP request to ProgressDB API.
        
        Args:
            path: Request path (e.g., '/frontend/v1/threads')
            method: HTTP method
            body: Optional request body
            user_id: Optional user ID for X-User-ID header
            
        Returns:
            Parsed JSON response, text, or None for 204 responses
            
        Raises:
            Exception: On request failure or API errors
        """
        url = f"{self.base_url}{path}"
        has_body = body is not None and method not in ['GET', 'DELETE']
        headers = self._headers(user_id, has_body)
        
        # Use custom fetch if provided
        if self.fetch:
            return self._custom_fetch(url, method, body, headers)
            
        # Use requests library
        try:
            response = requests.request(
                method,
                url,
                headers=headers,
                json=body if has_body else None,
                timeout=self.timeout
            )
            
            # Handle 204 No Content
            if response.status_code == 204:
                return None
                
            # Handle error responses
            if not response.ok:
                content_type = response.headers.get('content-type', '')
                if 'application/json' in content_type:
                    error_data = response.json()
                    raise ApiError(response.status_code, error_data)
                raise ApiError(response.status_code, {'error': f'HTTP {response.status_code}', 'message': response.text})
                
            # Parse successful responses
            content_type = response.headers.get('content-type', '')
            if 'application/json' in content_type:
                return response.json()
            return response.text
            
        except requests.RequestException as e:
            raise Exception(f'Request failed: {str(e)}')

    def _custom_fetch(self, url: str, method: str, body: Any, headers: Dict[str, str]) -> Union[Dict[str, Any], str, None]:
        """Use custom fetch function if provided."""
        # This would need to be implemented based on the custom fetch interface
        # For now, raise NotImplementedError
        raise NotImplementedError("Custom fetch function not implemented")

    @staticmethod
    def encode_url_component(component: str) -> str:
        """Encode URL component (similar to encodeURIComponent in JS).
        
        Args:
            component: String to encode
            
        Returns:
            URL-encoded string
        """
        return quote(component, safe='')