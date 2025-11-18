"""Simple unit tests for the new ProgressDB Python SDK - no server required."""

import pytest
from unittest.mock import AsyncMock


def test_client_import():
    """Test that we can import the client."""
    try:
        from progressdb import ProgressDBClient, SDKOptions
        assert True
    except ImportError:
        pytest.fail("Failed to import ProgressDBClient")


def test_client_instantiation_frontend_mode():
    """Test client instantiation in frontend mode."""
    from progressdb import ProgressDBClient, SDKOptions
    
    options: SDKOptions = {
        "mode": "frontend",
        "base_url": "https://api.example.com",
        "api_key": "test-key"
    }
    
    client = ProgressDBClient(options)
    
    assert client.mode == "frontend"
    assert client.base_url == "https://api.example.com"
    assert client.api_key == "test-key"
    assert client.http_client is not None
    assert client.health_service is not None
    assert client.threads_service is not None
    assert client.messages_service is not None


def test_client_instantiation_backend_mode():
    """Test client instantiation in backend mode."""
    from progressdb import ProgressDBClient, SDKOptions
    
    options: SDKOptions = {
        "mode": "backend",
        "base_url": "https://api.example.com",
        "api_key": "test-key"
    }
    
    client = ProgressDBClient(options)
    
    assert client.mode == "backend"
    assert client.base_url == "https://api.example.com"
    assert client.api_key == "test-key"


def test_client_instantiation_defaults():
    """Test client instantiation with defaults."""
    from progressdb import ProgressDBClient
    
    client = ProgressDBClient()
    
    assert client.mode == "frontend"  # Default mode
    assert client.base_url == "http://localhost:8080"  # Default base URL
    assert client.api_key is None  # No default API key


def test_client_has_all_methods():
    """Test that client has all expected methods."""
    from progressdb import ProgressDBClient
    
    client = ProgressDBClient()
    
    # Health methods
    assert hasattr(client, 'healthz')
    assert hasattr(client, 'readyz')
    assert callable(client.healthz)
    assert callable(client.readyz)
    
    # Thread methods
    assert hasattr(client, 'create_thread')
    assert hasattr(client, 'list_threads')
    assert hasattr(client, 'get_thread')
    assert hasattr(client, 'update_thread')
    assert hasattr(client, 'delete_thread')
    
    # Message methods
    assert hasattr(client, 'list_thread_messages')
    assert hasattr(client, 'create_thread_message')
    assert hasattr(client, 'get_thread_message')
    assert hasattr(client, 'update_thread_message')
    assert hasattr(client, 'delete_thread_message')
    
    # Cache management methods
    assert hasattr(client, 'clear_signature_cache')
    assert hasattr(client, 'get_cache_stats')


def test_services_instantiation():
    """Test that services are properly instantiated."""
    from progressdb import ProgressDBClient
    
    client = ProgressDBClient()
    
    # Check services exist and have expected methods
    assert client.health_service is not None
    assert hasattr(client.health_service, 'healthz')
    assert hasattr(client.health_service, 'readyz')
    
    assert client.threads_service is not None
    assert hasattr(client.threads_service, 'create_thread')
    assert hasattr(client.threads_service, 'list_threads')
    assert hasattr(client.threads_service, 'get_thread')
    assert hasattr(client.threads_service, 'update_thread')
    assert hasattr(client.threads_service, 'delete_thread')
    
    assert client.messages_service is not None
    assert hasattr(client.messages_service, 'list_thread_messages')
    assert hasattr(client.messages_service, 'create_thread_message')
    assert hasattr(client.messages_service, 'get_thread_message')
    assert hasattr(client.messages_service, 'update_thread_message')
    assert hasattr(client.messages_service, 'delete_thread_message')


def test_cache_methods_exist():
    """Test that cache management methods exist."""
    from progressdb import ProgressDBClient, SDKOptions
    
    # Test frontend mode (cache methods should still exist but do nothing)
    frontend_options: SDKOptions = {"mode": "frontend"}
    frontend_client = ProgressDBClient(frontend_options)
    
    assert hasattr(frontend_client, 'clear_signature_cache')
    assert hasattr(frontend_client, 'get_cache_stats')
    assert callable(frontend_client.clear_signature_cache)
    assert callable(frontend_client.get_cache_stats)
    
    # Test backend mode
    backend_options: SDKOptions = {"mode": "backend"}
    backend_client = ProgressDBClient(backend_options)
    
    assert hasattr(backend_client, 'clear_signature_cache')
    assert hasattr(backend_client, 'get_cache_stats')
    assert callable(backend_client.clear_signature_cache)
    assert callable(backend_client.get_cache_stats)


def test_type_imports():
    """Test that all types can be imported."""
    try:
        from progressdb import (
            SDKOptions,
            ApiErrorResponse,
            MessageType,
            MessageCreateRequest,
            MessageUpdateRequest,
            PaginationResponse,
            ThreadListQuery,
            MessageListQuery,
            HealthzResponse,
            ReadyzResponse,
            ThreadResponse,
            ThreadsListResponse,
            CreateThreadResponse,
            UpdateThreadResponse,
            DeleteThreadResponse,
            MessageResponse,
            MessagesListResponse,
            CreateMessageResponse,
            UpdateMessageResponse,
            DeleteMessageResponse,
            ThreadType,
            ThreadCreateRequest,
            ThreadUpdateRequest,
        )
        assert True
    except ImportError as e:
        pytest.fail(f"Failed to import types: {e}")


def test_http_client_instantiation():
    """Test that HTTP client is properly instantiated."""
    from progressdb import ProgressDBClient, SDKOptions
    
    options: SDKOptions = {
        "mode": "frontend",
        "base_url": "https://test.example.com",
        "api_key": "test-key",
        "default_user_id": "user123",
        "default_user_signature": "sig123"
    }
    
    client = ProgressDBClient(options)
    
    assert client.http_client is not None
    assert client.http_client.mode == "frontend"
    assert client.http_client.base_url == "https://test.example.com"
    assert client.http_client.api_key == "test-key"
    assert client.http_client.default_user_id == "user123"
    assert client.http_client.default_user_signature == "sig123"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])