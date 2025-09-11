import { httpRequest } from './http';
import { ApiError } from './errors';
import { Message, Thread } from './types';

export type BackendClientOptions = {
  baseUrl: string;
  apiKey: string;
  timeoutMs?: number;
  maxRetries?: number;
};

export class BackendClient {
  baseUrl: string;
  apiKey: string;
  timeoutMs?: number;
  maxRetries?: number;

  constructor(opts: BackendClientOptions) {
    this.baseUrl = opts.baseUrl;
    this.apiKey = opts.apiKey;
    this.timeoutMs = opts.timeoutMs;
    this.maxRetries = opts.maxRetries;
  }

  private headers() {
    return {
      Authorization: `Bearer ${this.apiKey}`,
    } as Record<string,string>;
  }

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

  // signing helper
  async signUser(userId: string): Promise<{ userId: string; signature: string }> {
    const res = await this.request<{ userId: string; signature: string }>('POST', '/v1/_sign', { userId });
    return res;
  }

  // admin endpoints
  async adminHealth(): Promise<{ status: string; service?: string }> {
    return await this.request('GET', '/admin/health');
  }

  async adminStats(): Promise<{ threads: number; messages: number }> {
    return await this.request('GET', '/admin/stats');
  }

  // threads
  // Accept optional query filters. For backend callers the server requires
  // an author to be supplied (either via signature or via query/header).
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

  async getThread(id: string, author: string): Promise<Thread> {
    if (!author) throw new Error('author is required for backend getThread calls');
    const path = `/v1/threads/${encodeURIComponent(id)}`;
    return await this.request<Thread>('GET', path, undefined, { 'X-User-ID': author });
  }

  async deleteThread(id: string, author: string): Promise<void> {
    if (!author) throw new Error('author is required for backend deleteThread calls');
    await this.request('DELETE', `/v1/threads/${encodeURIComponent(id)}`, undefined, { 'X-User-ID': author });
  }

  // low-level helpers
  async createThread(t: Partial<Thread>, author: string): Promise<Thread> {
    if (!author) throw new Error('author is required for backend createThread calls');
    return await this.request<Thread>('POST', '/v1/threads', t, { 'X-User-ID': author });
  }

  async updateThread(id: string, t: Partial<Thread>, author: string): Promise<Thread> {
    if (!author) throw new Error('author is required for backend updateThread calls');
    return await this.request<Thread>('PUT', `/v1/threads/${encodeURIComponent(id)}`, t, { 'X-User-ID': author });
  }

  async createMessage(m: Partial<Message>, author: string): Promise<Message> {
    if (!author) throw new Error('author is required for backend createMessage calls');
    return await this.request<Message>('POST', '/v1/messages', m, { 'X-User-ID': author });
  }
}

export default BackendClient;
