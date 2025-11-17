import { HTTPClient } from '../client/http';
import type { HealthzResponse, ReadyzResponse } from '../types';

export class HealthService {
  private httpClient: HTTPClient;

  constructor(httpClient: HTTPClient) {
    this.httpClient = httpClient;
  }

  /**
   * Basic health check.
   * @returns parsed JSON health object from GET /healthz
   */
  healthz(): Promise<HealthzResponse> {
    return this.httpClient.request('/healthz', 'GET');
  }

  /**
   * Readiness check with version info.
   * @returns parsed JSON readiness object from GET /readyz
   */
  readyz(): Promise<ReadyzResponse> {
    return this.httpClient.request('/readyz', 'GET');
  }
}