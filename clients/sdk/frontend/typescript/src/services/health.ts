import { HTTPClient } from '../client/http';

export class HealthService {
  private httpClient: HTTPClient;

  constructor(httpClient: HTTPClient) {
    this.httpClient = httpClient;
  }

  /**
   * Basic health check.
   * @returns parsed JSON health object from GET /healthz
   */
  healthz(): Promise<{ status: string }> {
    return this.httpClient.request('/healthz', 'GET');
  }

  /**
   * Readiness check with version info.
   * @returns parsed JSON readiness object from GET /readyz
   */
  readyz(): Promise<{ status: string; version?: string }> {
    return this.httpClient.request('/readyz', 'GET');
  }
}