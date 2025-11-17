import { HTTPClient } from '../client/http';
import { validatePaginationQuery } from '../utils/validation';
import type { MessageCreateRequestType, MessageUpdateRequestType, MessageResponseType, MessagesListResponseType, CreateMessageResponseType, UpdateMessageResponseType, DeleteMessageResponseType, MessageListQueryType } from '../types';

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
  listThreadMessages(threadKey: string, query: MessageListQueryType = {}, userId?: string, userSignature?: string): Promise<MessagesListResponseType> {
    validatePaginationQuery(query);
    const qs = new URLSearchParams();
    if (query.limit !== undefined) qs.set('limit', String(query.limit));
    if (query.before) qs.set('before', query.before);
    if (query.after) qs.set('after', query.after);
    if (query.anchor) qs.set('anchor', query.anchor);
    if (query.sort_by) qs.set('sort_by', query.sort_by);
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages${qs.toString() ? `?${qs.toString()}` : ''}`, 'GET', undefined, userId, userSignature) as Promise<MessagesListResponseType>;
  }

  /**
   * Create a message within a thread.
   * @param threadKey thread key
   * @param msg message payload
   * @param userId optional user id to send as X-User-ID
   * @param userSignature optional signature to send as X-User-Signature
   */
  createThreadMessage(threadKey: string, msg: MessageCreateRequestType, userId?: string, userSignature?: string): Promise<CreateMessageResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages`, 'POST', msg, userId, userSignature) as Promise<CreateMessageResponseType>;
  }

  /**
   * Retrieve a message by key within a thread.
   * @param threadKey thread key
   * @param messageKey message key
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  getThreadMessage(threadKey: string, messageKey: string, userId?: string, userSignature?: string): Promise<MessageResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(messageKey)}`, 'GET', undefined, userId, userSignature) as Promise<MessageResponseType>;
  }

  /**
   * Update a message within a thread.
   * @param threadKey thread key
   * @param messageKey message key
   * @param msg message payload
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  updateThreadMessage(threadKey: string, messageKey: string, msg: MessageUpdateRequestType, userId?: string, userSignature?: string): Promise<UpdateMessageResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(messageKey)}`, 'PUT', msg, userId, userSignature) as Promise<UpdateMessageResponseType>;
  }

  /**
   * Soft-delete a message within a thread.
   * @param threadKey thread key
   * @param messageKey message key
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  deleteThreadMessage(threadKey: string, messageKey: string, userId?: string, userSignature?: string): Promise<DeleteMessageResponseType> {
    return this.httpClient.request(`/frontend/v1/threads/${encodeURIComponent(threadKey)}/messages/${encodeURIComponent(messageKey)}`, 'DELETE', undefined, userId, userSignature) as Promise<DeleteMessageResponseType>;
  }
}