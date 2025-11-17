/*
 ProgressDB Frontend TypeScript SDK
 - Uses fetch to call ProgressDB endpoints.
 - Designed for frontend callers using a frontend API key (sent as `X-API-Key`).
 - Requires callers to supply `userId` and `userSignature` which are sent as
   `X-User-ID` and `X-User-Signature` on protected endpoints.
 */

// Export all types
export * from './types';

// Import internal modules
import { HTTPClient } from './client/http';
import { HealthService } from './services/health';
import { MessagesService } from './services/messages';
import { ThreadsService } from './services/threads';
import type { SDKOptionsType, ThreadCreateRequestType, ThreadUpdateRequestType, MessageCreateRequestType, MessageUpdateRequestType, ThreadListQueryType, MessageListQueryType } from './types';

/**
 * ProgressDBClient is the frontend (browser) SDK for ProgressDB.
 * It wraps `fetch` and exposes convenience methods for threads, messages
 * and reactions. For protected endpoints callers must provide `userId`
 * and `userSignature` (sent as `X-User-ID` and `X-User-Signature`).
 */
export class ProgressDBClient {
  private httpClient: HTTPClient;
  private healthService: HealthService;
  private messagesService: MessagesService;
  private threadsService: ThreadsService;

  baseUrl: string;
  apiKey?: string;
  defaultUserId?: string;
  defaultUserSignature?: string;
  fetchImpl: typeof fetch;

  /**
   * Create a new ProgressDBClient.
   * @param opts SDK options (baseUrl, apiKey, defaultUserId, defaultUserSignature, fetch)
   */
  constructor(opts: SDKOptionsType = {}) {
    this.httpClient = new HTTPClient(opts);
    this.healthService = new HealthService(this.httpClient);
    this.messagesService = new MessagesService(this.httpClient);
    this.threadsService = new ThreadsService(this.httpClient);

    // Expose HTTP client properties for backward compatibility
    this.baseUrl = this.httpClient.baseUrl;
    this.apiKey = this.httpClient.apiKey;
    this.defaultUserId = this.httpClient.defaultUserId;
    this.defaultUserSignature = this.httpClient.defaultUserSignature;
    this.fetchImpl = this.httpClient.fetchImpl;
  }

  // Health endpoints
  /**
   * Basic health check.
   * @returns parsed JSON health object from GET /healthz
   */
  healthz(): Promise<{ status: string }> {
    return this.healthService.healthz();
  }

  /**
   * Readiness check with version info.
   * @returns parsed JSON readiness object from GET /readyz
   */
  readyz(): Promise<{ status: string; version?: string }> {
    return this.healthService.readyz();
  }

  // Messages - thread-scoped only per OpenAPI spec
  /**
   * List messages for a thread.
   * @param threadKey thread key
   * @param query optional query parameters (limit, before, after, anchor, sort_by)
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
   listThreadMessages(threadKey: string, query: MessageListQueryType = {}, userId?: string, userSignature?: string) {
    return this.messagesService.listThreadMessages(threadKey, query, userId, userSignature);
  }

  /**
   * Create a message within a thread.
   * @param threadKey thread key
   * @param msg message payload
   * @param userId optional user id to send as X-User-ID
   * @param userSignature optional signature to send as X-User-Signature
   */
   createThreadMessage(threadKey: string, msg: MessageCreateRequestType, userId?: string, userSignature?: string) {
    return this.messagesService.createThreadMessage(threadKey, msg, userId, userSignature);
  }

  /**
   * Retrieve a message by id within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  getThreadMessage(threadKey: string, id: string, userId?: string, userSignature?: string) {
    return this.messagesService.getThreadMessage(threadKey, id, userId, userSignature);
  }

  /**
   * Update a message within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param msg message payload
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
   updateThreadMessage(threadKey: string, id: string, msg: MessageUpdateRequestType, userId?: string, userSignature?: string) {
    return this.messagesService.updateThreadMessage(threadKey, id, msg, userId, userSignature);
  }

  /**
   * Soft-delete a message within a thread.
   * @param threadKey thread key
   * @param id message id
   * @param userId optional user id to attach as X-User-ID
   * @param userSignature optional signature to attach as X-User-Signature
   */
  deleteThreadMessage(threadKey: string, id: string, userId?: string, userSignature?: string) {
    return this.messagesService.deleteThreadMessage(threadKey, id, userId, userSignature);
  }

  // Threads
  /**
   * Create a new thread.
   * @param thread thread payload with required title
   * @param userId optional user id
   * @param userSignature optional signature
   */
   createThread(thread: ThreadCreateRequestType, userId?: string, userSignature?: string) {
    return this.threadsService.createThread(thread, userId, userSignature);
  }

  /**
   * List threads visible to the current user.
   * @param query optional query parameters (limit, before, after, anchor, sort_by)
   * @param userId optional user id
   * @param userSignature optional signature
   */
   listThreads(query: ThreadListQueryType = {}, userId?: string, userSignature?: string) {
    return this.threadsService.listThreads(query, userId, userSignature);
  }

  /**
   * Retrieve thread metadata by key.
   * @param threadKey thread key
   * @param userId optional user id
   * @param userSignature optional signature
   */
  getThread(threadKey: string, userId?: string, userSignature?: string) {
    return this.threadsService.getThread(threadKey, userId, userSignature);
  }

  /**
   * Soft-delete a thread by key.
   * @param threadKey thread key
   * @param userId optional user id
   * @param userSignature optional signature
   */
  deleteThread(threadKey: string, userId?: string, userSignature?: string) {
    return this.threadsService.deleteThread(threadKey, userId, userSignature);
  }

  /**
   * Update thread metadata.
   * @param threadKey thread key
   * @param thread partial thread payload (title)
   * @param userId optional user id
   * @param userSignature optional signature
   */
   updateThread(threadKey: string, thread: ThreadUpdateRequestType, userId?: string, userSignature?: string) {
    return this.threadsService.updateThread(threadKey, thread, userId, userSignature);
  }
}

// Re-export for backward compatibility
export { ThreadCreateRequestType, ThreadUpdateRequestType, MessageCreateRequestType, MessageUpdateRequestType } from './types';
export default ProgressDBClient;