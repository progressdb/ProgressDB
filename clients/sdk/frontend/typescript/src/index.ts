/*
 ProgressDB Frontend TypeScript SDK
 - Uses fetch to call ProgressDB endpoints.
 - Designed for frontend callers using a frontend API key (sent as `X-API-Key`).
 - Requires callers to supply `userId` and `userSignature` which are sent as
   `X-User-ID` and `X-User-Signature` on protected endpoints.
*/

export type Message = {
  id?: string;
  thread?: string;
  author?: string;
  role?: string; // e.g. "user" | "system"; defaults to "user" when omitted
  ts?: number;
  body?: any;
  reply_to?: string;
  deleted?: boolean;
  reactions?: Record<string, string>;
};

export type Thread = {
  id: string;
  title?: string;
  slug?: string;
  created_ts?: number;
  updated_ts?: number;
  author?: string;
  metadata?: Record<string, any>;
};

export type ReactionInput = { id: string; reaction: string };

export type SDKOptions = {
  baseUrl?: string;
  apiKey?: string; // frontend API key sent as X-API-Key
  defaultUserId?: string;
  defaultUserSignature?: string;
  fetch?: typeof fetch;
};

/**
 * Build request headers used by the frontend SDK.
 * @param apiKey frontend API key to send as `X-API-Key`
 * @param userId optional user id to send as `X-User-ID`
 * @param userSignature optional signature to send as `X-User-Signature`
 * @returns headers object
 */
function buildHeaders(apiKey?: string, userId?: string, userSignature?: string) {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  };
  if (apiKey) headers['X-API-Key'] = apiKey;
  if (userId) headers['X-User-ID'] = userId;
  if (userSignature) headers['X-User-Signature'] = userSignature;
  return headers;
}

/**
 * ProgressDBClient is the frontend (browser) SDK for ProgressDB.
 * It wraps `fetch` and exposes convenience methods for threads, messages
 * and reactions. For protected endpoints callers must provide `userId`
 * and `userSignature` (sent as `X-User-ID` and `X-User-Signature`).
 */
export class ProgressDBClient {
  baseUrl: string;
  apiKey?: string;
  defaultUserId?: string;
  defaultUserSignature?: string;
  fetchImpl: typeof fetch;

  /**
   * Create a new ProgressDBClient.
   * @param opts SDK options (baseUrl, apiKey, defaultUserId, defaultUserSignature, fetch)
   */
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
   * @param userId optional user id to attach as `X-User-ID`
   * @param userSignature optional signature to attach as `X-User-Signature`
   */
  private async request(path: string, method = 'GET', body?: any, userId?: string, userSignature?: string) {
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

  // Health
  /**
   * Health check.
   * @returns parsed JSON health object from GET /healthz
   */
  health(): Promise<any> {
    return this.request('/healthz', 'GET');
  }

  // Messages
  /**
   * List messages (optionally scoped to a thread via the `thread` query param).
   * @param query optional query parameters { thread, limit }
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  listMessages(query: { thread?: string; limit?: number } = {}, userId?: string, userSignature?: string): Promise<{ thread?: string; messages: Message[] }> {
    const qs = new URLSearchParams();
    if (query.thread) qs.set('thread', query.thread);
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    return this.request('/v1/messages' + (qs.toString() ? `?${qs.toString()}` : ''), 'GET', undefined, userId, userSignature) as Promise<{ thread?: string; messages: Message[] }>;
  }

  /**
   * Create a message. If `msg.thread` is omitted a new thread is created.
   * @param msg message payload
   * @param userId optional user id to send as X-User-ID
   * @param userSignature optional signature to send as X-User-Signature
   */
  createMessage(msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request('/v1/messages', 'POST', msg, userId, userSignature) as Promise<Message>;
  }

  // message-level get/update/delete removed; use thread-scoped APIs instead

  /**
   * List stored versions for a message under a thread.
   * GET /v1/threads/{threadID}/messages/{id}/versions
   */
  listMessageVersions(threadID: string, id: string, userId?: string, userSignature?: string): Promise<{ id: string; versions: Message[] }> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/versions`, 'GET', undefined, userId, userSignature) as Promise<{ id: string; versions: Message[] }>;
  }

  // Reactions (thread-scoped)
  /**
   * List reactions for a message in a thread.
   */
  listReactions(threadID: string, id: string, userId?: string, userSignature?: string): Promise<{ id: string; reactions: Array<{ id: string; reaction: string }> }> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/reactions`, 'GET', undefined, userId, userSignature) as Promise<{ id: string; reactions: Array<{ id: string; reaction: string }> }>;
  }

  /**
   * Add or update a reaction for a message in a thread.
   */
  addOrUpdateReaction(threadID: string, id: string, input: ReactionInput, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/reactions`, 'POST', input, userId, userSignature) as Promise<Message>;
  }

  /**
   * Remove a reaction by identity for a message within a thread.
   */
  removeReaction(threadID: string, id: string, identity: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}/reactions/${encodeURIComponent(identity)}`, 'DELETE', undefined, userId, userSignature);
  }

  // Threads
  /**
   * Create a new thread.
   * @param thread partial thread payload
   * @param userId optional user id
   * @param userSignature optional signature
   */
  createThread(thread: Partial<Thread>, userId?: string, userSignature?: string): Promise<Thread> {
    return this.request('/v1/threads', 'POST', thread, userId, userSignature) as Promise<Thread>;
  }

  /**
   * List threads visible to the current user.
   * @param userId optional user id
   * @param userSignature optional signature
   */
  listThreads(userId?: string, userSignature?: string): Promise<{ threads: Thread[] }> {
    return this.request('/v1/threads', 'GET', undefined, userId, userSignature) as Promise<{ threads: Thread[] }>;
  }

  /**
   * Retrieve thread metadata by id.
   * @param id thread id
   * @param userId optional user id
   * @param userSignature optional signature
   */
  getThread(id: string, userId?: string, userSignature?: string): Promise<Thread> {
    return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<Thread>;
  }

  /**
   * Soft-delete a thread by id.
   * @param id thread id
   * @param userId optional user id
   * @param userSignature optional signature
   */
  deleteThread(id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }

  /**
   * Update thread metadata.
   * @param id thread id
   * @param thread partial thread payload
   * @param userId optional user id
   * @param userSignature optional signature
   */
  updateThread(id: string, thread: Partial<Thread>, userId?: string, userSignature?: string): Promise<Thread> {
    return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'PUT', thread, userId, userSignature) as Promise<Thread>;
  }

  // Thread messages
  /**
   * Create a message within a thread.
   * @param threadID thread id
   * @param msg message payload
   * @param userId optional user id
   * @param userSignature optional signature
   */
  createThreadMessage(threadID: string, msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages`, 'POST', msg, userId, userSignature) as Promise<Message>;
  }

  /**
   * List messages for a thread.
   * @param threadID thread id
   * @param query optional query parameters (limit)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  listThreadMessages(threadID: string, query: { limit?: number } = {}, userId?: string, userSignature?: string): Promise<{ thread?: string; messages: Message[] }> {
    const qs = new URLSearchParams();
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<{ thread?: string; messages: Message[] }>;
  }

  /**
   * Retrieve a message by id within a thread.
   * @param threadID thread id
   * @param id message id
   * @param userId optional user id
   * @param userSignature optional signature
   */
  getThreadMessage(threadID: string, id: string, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<Message>;
  }

  /**
   * Update (append new version) a message within a thread.
   * @param threadID thread id
   * @param id message id
   * @param msg message payload
   * @param userId optional user id
   * @param userSignature optional signature
   */
  updateThreadMessage(threadID: string, id: string, msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature) as Promise<Message>;
  }

  /**
   * Soft-delete a message within a thread.
   * @param threadID thread id
   * @param id message id
   * @param userId optional user id
   * @param userSignature optional signature
   */
  deleteThreadMessage(threadID: string, id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }

  // Signing is admin-only; SDK exposes the call but it requires an admin key.
  /**
   * Create an HMAC signature for a user id using the server-side signing endpoint.
   * This is admin-only and requires an admin API key.
   * @param userIdToSign user id to sign
   */
  signUser(userIdToSign: string): Promise<{ userId: string; signature: string }> {
    return this.request('/v1/_sign', 'POST', { userId: userIdToSign }) as Promise<{ userId: string; signature: string }>;
  }
}

export default ProgressDBClient;
