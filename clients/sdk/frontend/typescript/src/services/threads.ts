import { HTTPClient } from '../client/http';
import type { ThreadCreateRequest, ThreadUpdateRequest, ThreadResponse, ThreadsListResponse } from '../types';

export class ThreadsService {
  private httpClient: HTTPClient;

  constructor(httpClient: HTTPClient) {
    this.httpClient = httpClient;
  }

  /**
   * Create a new thread.
   * @param thread thread payload with required title
   * @param userId optional user id
   * @param userSignature optional signature
   */
  createThread(thread: ThreadCreateRequest, userId?: string, userSignature?: string): Promise<{ key: string }> {
    return this.httpClient.request('/frontend/v1/threads', 'POST', thread, userId, userSignature) as Promise<{ key: string }>;
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
    return this.httpClient.request(`/frontend/v1/threads${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<ThreadsListResponse>;
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
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<ThreadResponse>;
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
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}${qs.toString() ? `?${qs.toString()}` : ''}`, 'DELETE', undefined, userId, userSignature);
  }

  /**
   * Update thread metadata.
   * @param threadKey thread key
   * @param thread partial thread payload (title, slug)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  updateThread(threadKey: string, thread: ThreadUpdateRequest, userId?: string, userSignature?: string): Promise<ThreadResponse> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}`, 'PUT', thread, userId, userSignature) as Promise<ThreadResponse>;
  }
}