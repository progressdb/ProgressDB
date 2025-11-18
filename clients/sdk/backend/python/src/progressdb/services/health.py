"""Health service for ProgressDB Python SDK."""

from typing import TYPE_CHECKING

from ..types.responses import HealthzResponseType, ReadyzResponseType

if TYPE_CHECKING:
    from ..client.http import HTTPClient


class HealthService:
    """Service for health check endpoints."""

    def __init__(self, http_client: "HTTPClient") -> None:
        """Initialize health service.
        
        Args:
            http_client: HTTP client instance
        """
        self.http_client = http_client

    async def healthz(self) -> HealthzResponseType:
        """Basic health check.
        
        Returns:
            Parsed JSON health object from GET /healthz
        """
        return await self.http_client.request("/healthz", "GET")

    async def readyz(self) -> ReadyzResponseType:
        """Readiness check with version info.
        
        Returns:
            Parsed JSON readiness object from GET /readyz
        """
        return await self.http_client.request("/readyz", "GET")