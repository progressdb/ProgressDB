import { httpRequest } from './http';
import { ApiError } from './errors';
import { Message, Thread } from './types';

export type BackendClientOptions = {
  baseUrl: string;
  apiKey: string;
  timeoutMs?: number;
  maxRetries?: number;
};

/**
 * BackendClient provides server-side helpers to call ProgressDB admin
 * and backend endpoints. It includes retry/timeout behavior and
 * attaches server-side authorization headers.
 */
export class BackendClient {
  baseUrl: string;
  apiKey: string;
  timeoutMs?: number;
  maxRetries?: number;

  /**
   * Create a new BackendClient.
   * @param opts configuration options including `baseUrl` and `apiKey`
   */
  constructor(opts: BackendClientOptions) {
    this.baseUrl = opts.baseUrl;
    this.apiKey = opts.apiKey;
    this.timeoutMs = opts.timeoutMs;
    this.maxRetries = opts.maxRetries;
  }

  /**
   * Build default headers for backend requests (authorization, etc.).
   * @returns headers object
   */
  private headers() {
    return {
      Authorization: `Bearer ${this.apiKey}`,
    } as Record<string,string>;
  }

  /**
   * Perform an HTTP request against the ProgressDB server.
   * @template T expected response type
   * @param method HTTP method (GET, POST, PUT, DELETE)
   * @param path URL path (should begin with `/`)
   * @param body optional request body (will be JSON-stringified)
   * @param extraHeaders optional additional headers to merge
   * @returns parsed response body as T
   * @throws ApiError on non-2xx responses or other network errors
   */
  async request<T>(method: string, path: string, body?: any, extraHeaders: Record<string,string> = {}): Promise<T> {
    try {
      const headers = Object.assign({}, this.headers(), extraHeaders || {});
      return await httpRequest<T>(this.baseUrl, method, path, body, headers, {
        timeoutMs: this.timeoutMs,
        maxRetries: this.maxRetries,
      });
    } catch (err) {
      if (err instanceof ApiError) throw err;
      throw err;
    }
  }

  /**
   * Create an HMAC signature for a user id using the server-side signing endpoint.
   * Backend callers must have appropriate permissions to call this endpoint.
   * @param userId user id to sign
   * @returns object { userId, signature }
   */
  async signUser(userId: string): Promise<{ userId: string; signature: string }> {
    const res = await this.request<{ userId: string; signature: string }>('POST', '/v1/_sign', { userId });
    return res;
  }

  // admin endpoints
  /**
   * Admin health check.
   * @returns health object, e.g. { status: 'ok' }
   */
  async adminHealth(): Promise<{ status: string; service?: string }> {
    return await this.request('GET', '/admin/health');
  }

  /**
   * Admin stats endpoint.
   * @returns counts such as { threads, messages }
   */
  async adminStats(): Promise<{ threads: number; messages: number }> {
    return await this.request('GET', '/admin/stats');
  }

  // threads
  // Accept optional query filters. For backend callers the server requires
  // an author to be supplied (either via signature or via query/header).
  /**
   * List threads for a specific author.
   *
   * Backend callers MUST supply `author`. The author is sent as `X-User-ID`.
   * Optional filters: `title` (substring) and `slug` (exact match).
   * Throws immediately if `author` is missing.
   */
  /**
   * List threads for a given author with optional filters.
   * @param opts options object { author, title?, slug? }
   */
  async listThreads(opts: { author: string; title?: string; slug?: string }): Promise<Thread[]> {
    if (!opts || !opts.author) throw new Error('author is required for backend listThreads calls');
    const qs = new URLSearchParams();
    qs.set('author', opts.author);
    if (opts.title) qs.set('title', opts.title);
    if (opts.slug) qs.set('slug', opts.slug);
    const path = '/v1/threads' + `?${qs.toString()}`;
    // prefer header transport for author (avoid leaking in logs); include as header
    const res = await this.request<{ threads: Thread[] }>('GET', path, undefined, { 'X-User-ID': opts.author });
    return res.threads || [];
  }

  /**
   * Retrieve thread metadata by id.
   *
   * Backend callers MUST supply `author` which is sent as `X-User-ID`.
   * The server will resolve and validate the author; mismatches will be rejected.
   */
  /**
   * Get thread metadata by id. Backend callers must provide an author (X-User-ID).
   * @param id thread id
   * @param author backend author id
   */
  async getThread(id: string, author: string): Promise<Thread> {
    if (!author) throw new Error('author is required for backend getThread calls');
    const path = `/v1/threads/${encodeURIComponent(id)}`;
    return await this.request<Thread>('GET', path, undefined, { 'X-User-ID': author });
  }

  /**
   * Soft-delete a thread by id.
   *
   * Backend callers MUST supply `author` (sent as `X-User-ID`).
   */
  /**
   * Soft-delete a thread by id.
   * @param id thread id
   * @param author backend author id
   */
  async deleteThread(id: string, author: string): Promise<void> {
    if (!author) throw new Error('author is required for backend deleteThread calls');
    await this.request('DELETE', `/v1/threads/${encodeURIComponent(id)}`, undefined, { 'X-User-ID': author });
  }

  // low-level helpers
  /**
   * Create a new thread.
   *
   * Backend callers MUST supply `author` which is sent as `X-User-ID`.
   * The server will generate the thread id/slug and assign timestamps.
   */
  /**
   * Create a new thread. Backend callers must provide an author (X-User-ID).
   * @param t partial thread payload
   * @param author backend author id
   */
  async createThread(t: Partial<Thread>, author: string): Promise<Thread> {
    if (!author) throw new Error('author is required for backend createThread calls');
    return await this.request<Thread>('POST', '/v1/threads', t, { 'X-User-ID': author });
  }

  /**
   * Update thread metadata (title, etc.).
   *
   * Backend callers MUST supply `author` (sent as `X-User-ID`).
   */
  /**
   * Update thread metadata.
   * @param id thread id
   * @param t partial thread payload
   * @param author backend author id
   */
  async updateThread(id: string, t: Partial<Thread>, author: string): Promise<Thread> {
    if (!author) throw new Error('author is required for backend updateThread calls');
    return await this.request<Thread>('PUT', `/v1/threads/${encodeURIComponent(id)}`, t, { 'X-User-ID': author });
  }

  /**
   * Create a message. Backend callers MUST supply `author` (sent as `X-User-ID`).
   * The server will generate the message id and timestamps.
   */
  /**
   * Create a message (server generates id and ts).
   * @param m message payload
   * @param author backend author id
   */
  async createMessage(m: Partial<Message>, author: string): Promise<Message> {
    if (!author) throw new Error('author is required for backend createMessage calls');
    return await this.request<Message>('POST', '/v1/messages', m, { 'X-User-ID': author });
  }

  /**
   * List messages in a thread.
   * Optional query: { limit }
   */
  async listThreadMessages(threadID: string, opts: { limit?: number } = {}, author?: string): Promise<{ thread?: string; messages: Message[] }> {
    const qs = new URLSearchParams();
    if (opts.limit !== undefined) qs.set('limit', String(opts.limit));
    const path = `/v1/threads/${encodeURIComponent(threadID)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`;
    const headers = author ? { 'X-User-ID': author } : {};
    return await this.request<{ thread?: string; messages: Message[] }>('GET', path, undefined, headers);
  }

  /**
   * Get a single message by id within a thread.
   * @param threadID thread id to scope the message
   * @param id message id
   * @param author optional backend author id to send as X-User-ID
   */
  async getThreadMessage(threadID: string, id: string, author?: string): Promise<Message> {
    const headers = author ? { 'X-User-ID': author } : {};
    return await this.request<Message>('GET', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, undefined, headers);
  }

  /**
   * Update (append new version) a message within a thread.
   * @param threadID thread id
   * @param id message id
   * @param msg partial message payload
   * @param author optional backend author id to send as X-User-ID
   */
  async updateThreadMessage(threadID: string, id: string, msg: Partial<Message>, author?: string): Promise<Message> {
    const headers = author ? { 'X-User-ID': author } : {};
    return await this.request<Message>('PUT', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, msg, headers);
  }

  /**
   * Soft-delete a message within a thread (append tombstone).
   * @param threadID thread id
   * @param id message id
   * @param author optional backend author id to send as X-User-ID
   */
  async deleteThreadMessage(threadID: string, id: string, author?: string): Promise<void> {
    const headers = author ? { 'X-User-ID': author } : {};
    await this.request('DELETE', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, undefined, headers);
  }

  // Message versions + reactions (thread-scoped)
  /**
   * List all stored versions for a message id under a thread.
   * @param threadID thread id
   * @param id message id
   * @param author optional backend author id
   */
  async listMessageVersions(threadID: string, id: string, author?: string): Promise<{ id: string; versions: Message[] }> {
    const headers = author ? { 'X-User-ID': author } : {};
    return await this.request<{ id: string; versions: Message[] }>('GET', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/versions`, undefined, headers);
  }

  /**
   * List reactions on a message within a thread.
   * @param threadID thread id
   * @param id message id
   * @param author optional backend author id
   */
  async listReactions(threadID: string, id: string, author?: string): Promise<{ id: string; reactions: Array<{ id: string; reaction: string }> }> {
    const headers = author ? { 'X-User-ID': author } : {};
    return await this.request<{ id: string; reactions: Array<{ id: string; reaction: string }> }>('GET', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/reactions`, undefined, headers);
  }

  /**
   * Add or update a reaction for a message within a thread.
   * @param threadID thread id
   * @param id message id
   * @param input reaction record: { id, reaction }
   * @param author optional backend author id
   */
  async addOrUpdateReaction(threadID: string, id: string, input: { id: string; reaction: string }, author?: string): Promise<Message> {
    const headers = author ? { 'X-User-ID': author } : {};
    return await this.request<Message>('POST', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/reactions`, input, headers);
  }

  /**
   * Remove a reaction for a message within a thread.
   * @param threadID thread id
   * @param id message id
   * @param identity reactor identity
   * @param author optional backend author id
   */
  async removeReaction(threadID: string, id: string, identity: string, author?: string): Promise<void> {
    const headers = author ? { 'X-User-ID': author } : {};
    await this.request('DELETE', `/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/reactions/${encodeURIComponent(identity)}`, undefined, headers);
  }
}

export default BackendClient;
