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

  async request<T>(method: string, path: string, body?: any): Promise<T> {
    try {
      return await httpRequest<T>(this.baseUrl, method, path, body, this.headers(), {
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
  async listThreads(opts: { author?: string; title?: string; slug?: string } = {}): Promise<Thread[]> {
    const qs = new URLSearchParams();
    if (opts.author) qs.set('author', opts.author);
    if (opts.title) qs.set('title', opts.title);
    if (opts.slug) qs.set('slug', opts.slug);
    const path = '/v1/threads' + (qs.toString() ? `?${qs.toString()}` : '');
    const res = await this.request<{ threads: Thread[] }>('GET', path);
    return res.threads || [];
  }

  async deleteThread(id: string): Promise<void> {
    await this.request('DELETE', `/v1/threads/${encodeURIComponent(id)}`);
  }

  // low-level helpers
  async createThread(t: Partial<Thread>): Promise<Thread> {
    return await this.request<Thread>('POST', '/v1/threads', t);
  }

  async updateThread(id: string, t: Partial<Thread>): Promise<Thread> {
    return await this.request<Thread>('PUT', `/v1/threads/${encodeURIComponent(id)}`, t);
  }

  async createMessage(m: Partial<Message>): Promise<Message> {
    return await this.request<Message>('POST', '/v1/messages', m);
  }
}

export default BackendClient;
