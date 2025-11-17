import { buildHeaders } from './auth';
import type { SDKOptionsType, ApiErrorResponseType } from '../types';

export class HTTPClient {
  baseUrl: string;
  apiKey?: string;
  defaultUserId?: string;
  defaultUserSignature?: string;
  fetchImpl: typeof fetch;

  constructor(opts: SDKOptionsType = {}) {
    this.baseUrl = opts.baseUrl || '';
    this.apiKey = opts.apiKey;
    this.defaultUserId = opts.defaultUserId;
    this.defaultUserSignature = opts.defaultUserSignature;
    this.fetchImpl = opts.fetch || (typeof fetch !== 'undefined' ? fetch.bind(globalThis) : (() => { throw new Error('fetch not available, provide a fetch implementation in SDKOptions'); })());
  }

  /**
   * Build headers for a request using provided or default user credentials.
   */
  private headers(userId?: string, userSignature?: string, hasBody: boolean = true) {
    return buildHeaders(this.apiKey, userId || this.defaultUserId, userSignature || this.defaultUserSignature, hasBody);
  }

  /**
   * Internal helper to perform a fetch request against the configured base URL.
   * Returns parsed JSON when Content-Type is application/json, otherwise returns text.
   * @param path request path
   * @param method HTTP method
   * @param body optional request body
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  async request(path: string, method = 'GET', body?: any, userId?: string, userSignature?: string) {
    const url = this.baseUrl.replace(/\/$/, '') + path;
    const hasBody = body !== undefined && method !== 'GET' && method !== 'DELETE';
    const res = await this.fetchImpl(url, {
      method,
      headers: this.headers(userId, userSignature, hasBody),
      body: hasBody ? JSON.stringify(body) : undefined
    });
    if (res.status === 204) return null;
    
    // Handle error responses
    if (!res.ok) {
      const contentType = res.headers.get('content-type') || '';
      if (contentType.includes('application/json')) {
        const errorData = await res.json() as ApiErrorResponseType;
        throw new Error(errorData.error?.message || errorData.error?.error || 'API request failed');
      }
      throw new Error(`HTTP ${res.status}: ${res.statusText}`);
    }
    
    const contentType = res.headers.get('content-type') || '';
    if (contentType.includes('application/json')) return res.json();
    return res.text();
  }
}