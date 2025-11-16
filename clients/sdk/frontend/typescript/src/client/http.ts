import { buildHeaders } from './auth';
import type { SDKOptions } from '../types';

export class HTTPClient {
  baseUrl: string;
  apiKey?: string;
  defaultUserId?: string;
  defaultUserSignature?: string;
  fetchImpl: typeof fetch;

  constructor(opts: SDKOptions = {}) {
    this.baseUrl = opts.baseUrl || '';
    this.apiKey = opts.apiKey;
    this.defaultUserId = opts.defaultUserId;
    this.defaultUserSignature = opts.defaultUserSignature;
    this.fetchImpl = opts.fetch || (typeof fetch !== 'undefined' ? fetch.bind(globalThis) : (() => { throw new Error('fetch not available, provide a fetch implementation in SDKOptions'); })());
  }

  /**
   * Build headers for a request using provided or default user credentials.
   */
  private headers(userId?: string, userSignature?: string) {
    return buildHeaders(this.apiKey, userId || this.defaultUserId, userSignature || this.defaultUserSignature);
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
    const res = await this.fetchImpl(url, {
      method,
      headers: this.headers(userId, userSignature),
      body: body ? JSON.stringify(body) : undefined
    });
    if (res.status === 204) return null;
    const contentType = res.headers.get('content-type') || '';
    if (contentType.includes('application/json')) return res.json();
    return res.text();
  }
}