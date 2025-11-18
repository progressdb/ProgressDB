"""
Simple initialization and method existence tests for ProgressDB Python SDK.
No server required - just checks client setup and available methods.
"""

import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from progressdb import ProgressDB, ProgressDBClient, SDKOptionsType


def test_client_initialization():
    """Test that client initializes correctly with options."""
    options: SDKOptionsType = {
        "baseUrl": "https://api.test.com",
        "apiKey": "test-backend-key",
        "timeout": 5
    }
    
    # Test factory function
    client = ProgressDB(options)
    assert isinstance(client, ProgressDBClient)
    assert client.base_url == "https://api.test.com"
    assert client.api_key == "test-backend-key"
    assert client.http.timeout == 5
    
    # Test direct instantiation
    direct_client = ProgressDBClient(options)
    assert direct_client.base_url == "https://api.test.com"
    assert direct_client.api_key == "test-backend-key"
    
    print("PASS: Client initialization works")


def test_method_existence():
    """Test that all expected methods exist on the client."""
    options: SDKOptionsType = {
        "baseUrl": "https://api.test.com",
        "apiKey": "test-key"
    }
    client = ProgressDB(options)
    
    # Health methods
    assert hasattr(client, 'healthz'), "Missing healthz method"
    assert hasattr(client, 'readyz'), "Missing readyz method"
    assert callable(getattr(client, 'healthz')), "healthz is not callable"
    assert callable(getattr(client, 'readyz')), "readyz is not callable"
    
    # Thread methods
    thread_methods = [
        'create_thread', 'list_threads', 'get_thread', 
        'update_thread', 'delete_thread'
    ]
    for method in thread_methods:
        assert hasattr(client, method), f"Missing {method} method"
        assert callable(getattr(client, method)), f"{method} is not callable"
    
    # Message methods
    message_methods = [
        'create_thread_message', 'list_thread_messages', 'get_thread_message',
        'update_thread_message', 'delete_thread_message'
    ]
    for method in message_methods:
        assert hasattr(client, method), f"Missing {method} method"
        assert callable(getattr(client, method)), f"{method} is not callable"
    
    # Signature methods
    assert hasattr(client, 'sign_user'), "Missing sign_user method"
    assert callable(getattr(client, 'sign_user')), "sign_user is not callable"
    
    print("✅ All required methods exist")


def test_http_client_headers():
    """Test that HTTP client sets headers correctly."""
    from progressdb.http import HTTPClient
    
    # Test with API key and user ID
    opts = {
        "baseUrl": "https://api.test.com",
        "apiKey": "test-key"
    }
    http_client = HTTPClient(opts)
    
    # Test headers with body
    headers = http_client._headers("user-123", has_body=True)
    assert headers['Authorization'] == 'Bearer test-key'
    assert headers['X-User-ID'] == 'user-123'
    assert headers['Content-Type'] == 'application/json'
    
    # Test headers without body
    headers_no_body = http_client._headers("user-123", has_body=False)
    assert headers_no_body['Authorization'] == 'Bearer test-key'
    assert headers_no_body['X-User-ID'] == 'user-123'
    assert 'Content-Type' not in headers_no_body
    
    # Test headers without user ID
    headers_no_user = http_client._headers(None, has_body=True)
    assert headers_no_user['Authorization'] == 'Bearer test-key'
    assert 'X-User-ID' not in headers_no_user
    assert headers_no_user['Content-Type'] == 'application/json'
    
    print("✅ HTTP client headers work correctly")


def test_url_encoding():
    """Test URL encoding functionality."""
    from progressdb.http import HTTPClient
    
    # Test basic encoding
    encoded = HTTPClient.encode_url_component("hello world")
    assert encoded == "hello%20world"
    
    # Test special characters
    encoded = HTTPClient.encode_url_component("test & symbols!")
    assert encoded == "test%20%26%20symbols%21"
    
    # Test empty string
    encoded = HTTPClient.encode_url_component("")
    assert encoded == ""
    
    print("✅ URL encoding works correctly")


def test_type_imports():
    """Test that all types can be imported."""
    try:
        from progressdb import (
            SDKOptionsType,
            MessageType,
            ThreadType,
            ThreadCreateRequestType,
            ThreadUpdateRequestType,
            MessageCreateRequestType,
            MessageUpdateRequestType,
            ThreadListQueryType,
            MessageListQueryType,
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
            ApiErrorResponseType,
        )
        print("✅ All types import successfully")
    except ImportError as e:
        print(f"❌ Type import failed: {e}")
        raise


def test_error_imports():
    """Test that error classes can be imported."""
    try:
        from progressdb import (
            ProgressDBError,
            ApiError,
            NetworkError,
            ValidationError,
        )
        
        # Test error instantiation
        api_error = ApiError(400, {"error": "test error"})
        assert api_error.status_code == 400
        assert api_error.message == "test error"
        
        print("✅ Error classes import and work correctly")
    except ImportError as e:
        print(f"❌ Error import failed: {e}")
        raise


if __name__ == "__main__":
    print("🧪 Running ProgressDB Python SDK initialization tests...\n")
    
    test_client_initialization()
    test_method_existence()
    test_http_client_headers()
    test_url_encoding()
    test_type_imports()
    test_error_imports()
    
    print("\n🎉 All tests passed! SDK is properly initialized and ready to use.")