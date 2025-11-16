import { HTTPClient } from '../client/http';
import type { MessageCreateRequest, MessageUpdateRequest, MessageResponse, MessagesListResponse } from '../types';

export class MessagesService {
  private httpClient: HTTPClient;

  constructor(httpClient: HTTPClient) {
    this.httpClient = httpClient;
  }

  /**
   * List messages for a thread.
   * @param threadKey thread key
   * @param query optional query parameters (limit, before, after, anchor, sort_by)
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  listThreadMessages(threadKey: string, query: { limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string } = {}, userId?: string, userSignature?: string): Promise<MessagesListResponse> {
    const qs = new URLSearchParams();
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    if (query.before) qs.set('before', query.before);
    if (query.after) qs.set('after', query.after);
    if (query.anchor) qs.set('anchor', query.anchor);
    if (query.sort_by) qs.set('sort_by', query.sort_by);
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<MessagesListResponse>;
  }

  /**
   * Create a message within a thread.
   * @param threadKey thread key
   * @param msg message payload
   * @param userId optional user id to send as X-User-ID
   * @param userSignature optional signature to send as X-User-Signature
   */
  createThreadMessage(threadKey: string, msg: MessageCreateRequest, userId?: string, userSignature?: string): Promise<MessageResponse> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages`, 'POST', msg, userId, userSignature) as Promise<MessageResponse>;
  }

  /**
   * Retrieve a message by id within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  getThreadMessage(threadKey: string, id: string, userId?: string, userSignature?: string): Promise<MessageResponse> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(id)}`, 'GET', undefined, userId, userSignature) as Promise<MessageResponse>;
  }

  /**
   * Update a message within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param msg message payload
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  updateThreadMessage(threadKey: string, id: string, msg: MessageUpdateRequest, userId?: string, userSignature?: string): Promise<MessageResponse> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(id)}`, 'PUT', msg, userId, userSignature) as Promise<MessageResponse>;
  }

  /**
   * Soft-delete a message within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  deleteThreadMessage(threadKey: string, id: string, userId?: string, userSignature?: string): Promise<any> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(id)}`, 'DELETE', undefined, userId, userSignature);
  }
}