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

function buildHeaders(apiKey?: string, userId?: string, userSignature?: string) {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  };
  if (apiKey) headers['X-API-Key'] = apiKey;
  if (userId) headers['X-User-ID'] = userId;
  if (userSignature) headers['X-User-Signature'] = userSignature;
  return headers;
}

export class ProgressDBClient {
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

  private headers(userId?: string, userSignature?: string) {
    return buildHeaders(this.apiKey, userId || this.defaultUserId, userSignature || this.defaultUserSignature);
  }

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
  health(): Promise<any> {
    return this.request('/healthz', 'GET');
  }

  // Messages
  listMessages(query: { thread?: string; limit?: number } = {}, userId?: string, userSignature?: string): Promise<{ thread?: string; messages: Message[] }> {
    const qs = new URLSearchParams();
    if (query.thread) qs.set('thread', query.thread);
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    return this.request('/v1/messages' + (qs.toString() ? `?${qs.toString()}` : ''), 'GET', undefined, userId, userSignature) as Promise<{ thread?: string; messages: Message[] }>;
  }

  createMessage(msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request('/v1/messages', 'POST', msg, userId, userSignature) as Promise<Message>;
  }

  getMessage(id: string, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<Message>;
  }

  updateMessage(id: string, msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature) as Promise<Message>;
  }

  deleteMessage(id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }

  listMessageVersions(id: string, userId?: string, userSignature?: string): Promise<{ id: string; versions: Message[] }> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}/versions`, 'GET', undefined, userId, userSignature) as Promise<{ id: string; versions: Message[] }>;
  }

  // Reactions
  listReactions(id: string, userId?: string, userSignature?: string): Promise<{ id: string; reactions: Array<{ id: string; reaction: string }> }> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}/reactions`, 'GET', undefined, userId, userSignature) as Promise<{ id: string; reactions: Array<{ id: string; reaction: string }> }>;
  }

  addOrUpdateReaction(id: string, input: ReactionInput, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}/reactions`, 'POST', input, userId, userSignature) as Promise<Message>;
  }

  removeReaction(id: string, identity: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/messages/${encodeURIComponent(id)}/reactions/${encodeURIComponent(identity)}`, 'DELETE', undefined, userId, userSignature);
  }

  // Threads
  createThread(thread: Partial<Thread>, userId?: string, userSignature?: string): Promise<Thread> {
    return this.request('/v1/threads', 'POST', thread, userId, userSignature) as Promise<Thread>;
  }

  listThreads(userId?: string, userSignature?: string): Promise<{ threads: Thread[] }> {
    return this.request('/v1/threads', 'GET', undefined, userId, userSignature) as Promise<{ threads: Thread[] }>;
  }

  getThread(id: string, userId?: string, userSignature?: string): Promise<Thread> {
    return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<Thread>;
  }

  deleteThread(id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }

  updateThread(id: string, thread: Partial<Thread>, userId?: string, userSignature?: string): Promise<Thread> {
    return this.request(`/v1/threads/${encodeURIComponent(id)}`, 'PUT', thread, userId, userSignature) as Promise<Thread>;
  }

  // Thread messages
  createThreadMessage(threadID: string, msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages`, 'POST', msg, userId, userSignature) as Promise<Message>;
  }

  listThreadMessages(threadID: string, query: { limit?: number } = {}, userId?: string, userSignature?: string): Promise<{ thread?: string; messages: Message[] }> {
    const qs = new URLSearchParams();
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<{ thread?: string; messages: Message[] }>;
  }

  getThreadMessage(threadID: string, id: string, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<Message>;
  }

  updateThreadMessage(threadID: string, id: string, msg: Message, userId?: string, userSignature?: string): Promise<Message> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature) as Promise<Message>;
  }

  deleteThreadMessage(threadID: string, id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.request(`/v1/threads/${encodeURIComponent(threadID)}/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }

  // Signing is admin-only; SDK exposes the call but it requires an admin key.
  signUser(userIdToSign: string): Promise<{ userId: string; signature: string }> {
    return this.request('/v1/_sign', 'POST', { userId: userIdToSign }) as Promise<{ userId: string; signature: string }>;
  }
}

export default ProgressDBClient;
