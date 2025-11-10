/*
 ProgressDB Frontend TypeScript SDK
 - Uses fetch to call ProgressDB endpoints.
 - Designed for frontend callers using a frontend API key (sent as `X-API-Key`).
 - Requires callers to supply `userId` and `userSignature` which are sent as
   `X-User-ID` and `X-User-Signature` on protected endpoints.
*/

export type Message = {
  key?: string;
  thread?: string;
  author?: string;
  role?: string; // e.g. "user" | "system"; defaults to "user" when omitted
  created_ts?: number;
  updated_ts?: number;
  body?: any;
  reply_to?: string;
  deleted?: boolean;
};

export type Thread = {
  key: string;
  title?: string;
  slug?: string;
  created_ts?: number;
  updated_ts?: number;
  author?: string;
  deleted?: boolean;
  kms?: KMSMeta;
};

export type KMSMeta = {
  key_id?: string;
  wrapped_dek?: string;
  kek_id?: string;
  kek_version?: string;
};

export type ReactionInput = { id: string; reaction: string };

export type PaginationRequest = {
  before?: string;   // Fetch items older than this reference ID
  after?: string;    // Fetch items newer than this reference ID
  anchor?: string;   // Fetch items around this anchor (takes precedence if set)
  limit?: number;    // Max number to return
  sort_by?: string;  // Sort by field: "created_ts" or "updated_ts"
};

export type PaginationResponse = {
  before_anchor: string; // Use this to get previous page
  after_anchor: string;  // Use this to get next page
  has_before: boolean;   // True if there are items before BeforeAnchor (previous page exists)
  has_after: boolean;    // True if there are items after AfterAnchor (next page exists)
  count: number;        // Number of items returned in this page
  total: number;        // Total number of items available
};

export type ThreadsListResponse = {
  threads: Thread[];
  pagination?: PaginationResponse;
};

export type MessagesListResponse = {
  thread?: string;
  messages: Message[];
  pagination?: PaginationResponse;
};

export type ThreadResponse = {
  thread: Thread;
};

export type MessageResponse = {
  message: Message;
};

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
   * @returns parsed JSON health object from GET /admin/health
   */
  health(): Promise<any> {
    return this.request('/admin/health', 'GET');
  }

  // Messages - thread-scoped only per OpenAPI spec
  /**
   * List messages for a thread.
   * @param threadKey thread key
   * @param query optional query parameters (limit, before, after, anchor, sort_by, include_deleted)
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  listThreadMessages(threadKey: string, query: { limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string; include_deleted?: boolean } = {}, userId?: string, userSignature?: string): Promise<MessagesListResponse> {
    const qs = new URLSearchParams();
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    if (query.before) qs.set('before', query.before);
    if (query.after) qs.set('after', query.after);
    if (query.anchor) qs.set('anchor', query.anchor);
    if (query.sort_by) qs.set('sort_by', query.sort_by);
    if (query.include_deleted !== undefined) qs.set('include_deleted', String(query.include_deleted));
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<MessagesListResponse>;
  }

  /**
   * Create a message within a thread.
   * @param threadKey thread key
   * @param msg message payload
   * @param userId optional user id to send as X-User-ID
   * @param userSignature optional signature to send as X-User-Signature
   */
  createThreadMessage(threadKey: string, msg: Message, userId?: string, userSignature?: string): Promise<MessageResponse> {
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages`, 'POST', msg, userId, userSignature) as Promise<MessageResponse>;
  }

  /**
   * Retrieve a message by id within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  getThreadMessage(threadKey: string, id: string, userId?: string, userSignature?: string): Promise<MessageResponse> {
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<MessageResponse>;
  }

  /**
   * Update a message within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param msg message payload
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  updateThreadMessage(threadKey: string, id: string, msg: Message, userId?: string, userSignature?: string): Promise<MessageResponse> {
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature) as Promise<MessageResponse>;
  }

  /**
   * Soft-delete a message within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  deleteThreadMessage(threadKey: string, id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }

  // Reactions are not in the OpenAPI spec - removing these methods
  // If reactions are needed, they should be added to the OpenAPI spec first

  // Threads
  /**
   * Create a new thread.
   * @param thread thread payload with required title
   * @param userId optional user id
   * @param userSignature optional signature
   */
  createThread(thread: { title: string; slug?: string }, userId?: string, userSignature?: string): Promise<ThreadResponse> {
    return this.request('/frontend/v1/threads', 'POST', thread, userId, userSignature) as Promise<ThreadResponse>;
  }

  /**
   * List threads visible to the current user.
   * @param query optional query parameters (title, slug, limit, before, after, anchor, sort_by, author)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  listThreads(query: { title?: string; slug?: string; limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string; author?: string } = {}, userId?: string, userSignature?: string): Promise<ThreadsListResponse> {
    const qs = new URLSearchParams();
    if (query.title) qs.set('title', query.title);
    if (query.slug) qs.set('slug', query.slug);
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    if (query.before) qs.set('before', query.before);
    if (query.after) qs.set('after', query.after);
    if (query.anchor) qs.set('anchor', query.anchor);
    if (query.sort_by) qs.set('sort_by', query.sort_by);
    if (query.author) qs.set('author', query.author);
    return this.request(`/frontend/v1/threads${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<ThreadsListResponse>;
  }

  /**
   * Retrieve thread metadata by key.
   * @param threadKey thread key
   * @param query optional query parameters (author)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  getThread(threadKey: string, query: { author?: string } = {}, userId?: string, userSignature?: string): Promise<ThreadResponse> {
    const qs = new URLSearchParams();
    if (query.author) qs.set('author', query.author);
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<ThreadResponse>;
  }

  /**
   * Soft-delete a thread by key.
   * @param threadKey thread key
   * @param query optional query parameters (author)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  deleteThread(threadKey: string, query: { author?: string } = {}, userId?: string, userSignature?: string): Promise<any> {
    const qs = new URLSearchParams();
    if (query.author) qs.set('author', query.author);
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}${qs.toString() ? `?${qs.toString()}` : ''}`, 'DELETE', undefined, userId, userSignature);
  }

  /**
   * Update thread metadata.
   * @param threadKey thread key
   * @param thread partial thread payload (title, slug)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  updateThread(threadKey: string, thread: { title?: string; slug?: string }, userId?: string, userSignature?: string): Promise<ThreadResponse> {
    return this.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}`, 'PUT', thread, userId, userSignature) as Promise<ThreadResponse>;
  }

  // Backend signing endpoint - requires backend API key
  /**
   * Create an HMAC signature for a user id using the backend signing endpoint.
   * This requires a backend API key.
   * @param userIdToSign user id to sign
   */
  signUser(userIdToSign: string): Promise<{ userId: string; signature: string }> {
    return this.request('/backend/v1/sign', 'POST', { userId: userIdToSign }) as Promise<{ userId: string; signature: string }>;
  }
}

export default ProgressDBClient;
