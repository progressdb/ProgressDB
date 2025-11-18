import ProgressDBClient from '@progressdb/js';
import type { 
  SDKOptionsType, 
  ThreadCreateRequestType, 
  ThreadUpdateRequestType, 
  MessageCreateRequestType, 
  MessageUpdateRequestType,
  ThreadListQueryType,
  MessageListQueryType,
  CreateThreadResponseType,
  UpdateThreadResponseType,
  DeleteThreadResponseType,
  CreateMessageResponseType,
  UpdateMessageResponseType,
  DeleteMessageResponseType,
  ThreadResponseType,
  MessageResponseType,
  ThreadsListResponseType,
  MessagesListResponseType
} from '@progressdb/js';

export type BackendClientOptions = Omit<SDKOptionsType, 'defaultUserId' | 'defaultUserSignature'> & {
  timeoutMs?: number;
  maxRetries?: number;
};

interface CacheEntry {
  signature: string;
  expires: number;
}

/**
 * BackendClient provides server-side helpers to call ProgressDB endpoints.
 * It wraps the frontend TypeScript SDK and automatically handles signature generation
 * and caching for backend operations.
 * 
 * Key features:
 * - Automatic signature generation and caching (5-minute TTL)
 * - Uses backend API key authentication
 * - Reuses all frontend SDK logic (pagination, validation, types)
 * - Exposes signUser() method for external signature generation
 */
export class BackendClient {
  private frontendClient: ProgressDBClient;
  private signatureCache = new Map<string, CacheEntry>();
  private backendApiKey: string;

  /**
   * Create a new BackendClient.
   * @param opts configuration options including baseUrl and apiKey
   */
  constructor(opts: BackendClientOptions) {
    this.backendApiKey = opts.apiKey || '';
    
    // Initialize frontend SDK with same API key (will be used as X-API-Key)
    this.frontendClient = new ProgressDBClient({
      baseUrl: opts.baseUrl,
      apiKey: opts.apiKey,
      fetch: opts.fetch
    });
  }

  /**
   * Get cached signature or generate new one with 5-minute TTL.
   * @param userId user ID to generate signature for
   * @returns cached or fresh signature
   */
  private async getOrGenerateSignature(userId: string): Promise<string> {
    const cached = this.signatureCache.get(userId);
    const now = Date.now();
    
    // Return cached signature if still valid
    if (cached && cached.expires > now) {
      return cached.signature;
    }

    // Generate new signature
    const { signature } = await this.signUser(userId);
    
    // Cache for 5 minutes
    this.signatureCache.set(userId, {
      signature,
      expires: now + (5 * 60 * 1000) // 5 minutes TTL
    });
    
    return signature;
  }

  /**
   * Create an HMAC signature for a user id using the server-side signing endpoint.
   * Backend callers must have appropriate permissions to call this endpoint.
   * @param userId user id to sign
   * @returns object { userId, signature }
   */
  async signUser(userId: string): Promise<{ userId: string; signature: string }> {
    const url = `${this.frontendClient.baseUrl.replace(/\/$/, '')}/v1/_sign`;
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${this.backendApiKey}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ userId })
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to sign user: ${response.status} ${errorText}`);
    }

    return response.json();
  }

  /**
   * Clear signature cache for a specific user or all users.
   * @param userId optional user ID to clear, clears all if not provided
   */
  clearSignatureCache(userId?: string): void {
    if (userId) {
      this.signatureCache.delete(userId);
    } else {
      this.signatureCache.clear();
    }
  }

  /**
   * Get cache statistics for debugging/monitoring.
   * @returns object with cache size and entry details
   */
  getCacheStats(): { size: number; entries: Array<{ userId: string; expires: number }> } {
    const entries: Array<{ userId: string; expires: number }> = [];
    for (const [userId, entry] of this.signatureCache.entries()) {
      entries.push({ userId, expires: entry.expires });
    }
    return { size: this.signatureCache.size, entries };
  }

  // Thread operations - automatically handle signatures

  /**
   * Create a new thread.
   * @param thread thread payload with required title
   * @param userId user ID to create thread for
   */
  async createThread(thread: ThreadCreateRequestType, userId: string): Promise<CreateThreadResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.createThread(thread, userId, signature);
  }

  /**
   * List threads visible to the user.
   * @param query optional query parameters (limit, before, after, anchor, sort_by)
   * @param userId user ID to list threads for
   */
  async listThreads(query: ThreadListQueryType = {}, userId: string): Promise<ThreadsListResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.listThreads(query, userId, signature);
  }

  /**
   * Retrieve thread metadata by key.
   * @param threadKey thread key
   * @param userId user ID to access thread for
   */
  async getThread(threadKey: string, userId: string): Promise<ThreadResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.getThread(threadKey, userId, signature);
  }

  /**
   * Soft-delete a thread by key.
   * @param threadKey thread key
   * @param userId user ID to delete thread for
   */
  async deleteThread(threadKey: string, userId: string): Promise<DeleteThreadResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.deleteThread(threadKey, userId, signature);
  }

  /**
   * Update thread metadata.
   * @param threadKey thread key
   * @param thread partial thread payload (title)
   * @param userId user ID to update thread for
   */
  async updateThread(threadKey: string, thread: ThreadUpdateRequestType, userId: string): Promise<UpdateThreadResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.updateThread(threadKey, thread, userId, signature);
  }

  // Message operations - automatically handle signatures

  /**
   * List messages for a thread.
   * @param threadKey thread key
   * @param query optional query parameters (limit, before, after, anchor, sort_by)
   * @param userId user ID to list messages for
   */
  async listThreadMessages(threadKey: string, query: MessageListQueryType = {}, userId: string): Promise<MessagesListResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.listThreadMessages(threadKey, query, userId, signature);
  }

  /**
   * Create a message within a thread.
   * @param threadKey thread key
   * @param msg message payload
   * @param userId user ID to create message for
   */
  async createThreadMessage(threadKey: string, msg: MessageCreateRequestType, userId: string): Promise<CreateMessageResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.createThreadMessage(threadKey, msg, userId, signature);
  }

  /**
   * Retrieve a message by key within a thread.
   * @param threadKey thread key
   * @param messageKey message key
   * @param userId user ID to access message for
   */
  async getThreadMessage(threadKey: string, messageKey: string, userId: string): Promise<MessageResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.getThreadMessage(threadKey, messageKey, userId, signature);
  }

  /**
   * Update a message within a thread.
   * @param threadKey thread key
   * @param messageKey message key
   * @param msg message payload
   * @param userId user ID to update message for
   */
  async updateThreadMessage(threadKey: string, messageKey: string, msg: MessageUpdateRequestType, userId: string): Promise<UpdateMessageResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.updateThreadMessage(threadKey, messageKey, msg, userId, signature);
  }

  /**
   * Soft-delete a message within a thread.
   * @param threadKey thread key
   * @param messageKey message key
   * @param userId user ID to delete message for
   */
  async deleteThreadMessage(threadKey: string, messageKey: string, userId: string): Promise<DeleteMessageResponseType> {
    const signature = await this.getOrGenerateSignature(userId);
    return this.frontendClient.deleteThreadMessage(threadKey, messageKey, userId, signature);
  }

  // Health endpoints - no signature required

  /**
   * Basic health check.
   * @returns parsed JSON health object from GET /healthz
   */
  async healthz(): Promise<{ status: string }> {
    return this.frontendClient.healthz();
  }

  /**
   * Readiness check with version info.
   * @returns parsed JSON readiness object from GET /readyz
   */
  async readyz(): Promise<{ status: string; version?: string }> {
    return this.frontendClient.readyz();
  }
}

export default BackendClient;