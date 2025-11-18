"""
Error classes for ProgressDB Python SDK.

Simple error handling for backend client.
"""

from typing import Any, Dict, Optional


class ProgressDBError(Exception):
    """Base exception class for ProgressDB SDK."""
    
    def __init__(self, message: str, status_code: Optional[int] = None, response: Optional[Dict[str, Any]] = None):
        """Initialize ProgressDB error.
        
        Args:
            message: Error message
            status_code: HTTP status code (optional)
            response: Full error response from API (optional)
        """
        super().__init__(message)
        self.message = message
        self.status_code = status_code
        self.response = response


class ApiError(ProgressDBError):
    """API request error with status code and response details."""
    
    def __init__(self, status_code: int, response: Dict[str, Any]):
        """Initialize API error.
        
        Args:
            status_code: HTTP status code
            response: Error response from API
        """
        message = response.get('error', 'API request failed')
        if 'message' in response:
            message = f"{message}: {response['message']}"
        super().__init__(message, status_code, response)


class NetworkError(ProgressDBError):
    """Network-related error (connection timeout, DNS failure, etc.)."""
    
    def __init__(self, message: str):
        """Initialize network error.
        
        Args:
            message: Error message
        """
        super().__init__(message)


class ValidationError(ProgressDBError):
    """Input validation error."""
    
    def __init__(self, message: str):
        """Initialize validation error.
        
        Args:
            message: Error message
        """
        super().__init__(message)