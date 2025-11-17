import { HTTPClient } from '../client/http';
import { validatePaginationQuery } from '../utils/validation';
import type { ThreadCreateRequestType, ThreadUpdateRequestType, ThreadResponseType, ThreadsListResponseType, CreateThreadResponseType, UpdateThreadResponseType, DeleteThreadResponseType, ThreadListQueryType } from '../types';

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
  createThread(thread: ThreadCreateRequestType, userId?: string, userSignature?: string): Promise<CreateThreadResponseType> {
    return this.httpClient.request('/frontend/v1/threads', 'POST', thread, userId, userSignature) as Promise<CreateThreadResponseType>;
  }

  /**
   * List threads visible to the current user.
   * @param query optional query parameters (limit, before, after, anchor, sort_by)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  listThreads(query: ThreadListQueryType = {}, userId?: string, userSignature?: string): Promise<ThreadsListResponseType> {
    validatePaginationQuery(query);
    const qs = new URLSearchParams();
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    if (query.before) qs.set('before', query.before);
    if (query.after) qs.set('after', query.after);
    if (query.anchor) qs.set('anchor', query.anchor);
    if (query.sort_by) qs.set('sort_by', query.sort_by);
    return this.httpClient.request(`/frontend/v1/threads${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<ThreadsListResponseType>;
  }

  /**
   * Retrieve thread metadata by key.
   * @param threadKey thread key
   * @param userId optional user id
   * @param userSignature optional signature
   */
  getThread(threadKey: string, userId?: string, userSignature?: string): Promise<ThreadResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}`, 'GET', undefined, userId, userSignature) as Promise<ThreadResponseType>;
  }

  /**
   * Soft-delete a thread by key.
   * @param threadKey thread key
   * @param userId optional user id
   * @param userSignature optional signature
   */
  deleteThread(threadKey: string, userId?: string, userSignature?: string): Promise<DeleteThreadResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}`, 'DELETE', undefined, userId, userSignature) as Promise<DeleteThreadResponseType>;
  }

  /**
   * Update thread metadata.
   * @param threadKey thread key
   * @param thread partial thread payload (title)
   * @param userId optional user id
   * @param userSignature optional signature
   */
  updateThread(threadKey: string, thread: ThreadUpdateRequestType, userId?: string, userSignature?: string): Promise<UpdateThreadResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}`, 'PUT', thread, userId, userSignature) as Promise<UpdateThreadResponseType>;
  }
}