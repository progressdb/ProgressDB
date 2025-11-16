export type PaginationRequest = {
  before?: string;   // Fetch items older than this reference ID
  after?: string;    // Fetch items newer than this reference ID
  anchor?: string;   // Fetch items around this anchor (takes precedence if set)
  limit?: number;    // Max number to return
  sort_by?: string;  // Sort by field: "created_ts" or "updated_ts"
};

export type PaginationResponse = {
  before_anchor: string; // Use this to get previous page (pass as 'after' parameter)
  after_anchor: string;  // Use this to get next page (pass as 'before' parameter)
  has_before: boolean;   // True if there are items before before_anchor (previous page exists)
  has_after: boolean;    // True if there are items after after_anchor (next page exists)
  count: number;        // Number of items returned in this page
  total: number;        // Total number of items available
};

/**
 * Pagination navigation examples:
 * 
 * === THREADS (reverse chronological: [newest → oldest]) ===
 * 
 * // First page (newest threads)
 * const firstThreadsPage = await client.listThreads({ limit: 50 });
 * 
 * // Next page (older threads)
 * const olderThreads = await client.listThreads({ 
 *   after: firstThreadsPage.pagination.after_anchor 
 * });
 * 
 * // Previous page (newer threads) 
 * const newerThreads = await client.listThreads({ 
 *   before: firstThreadsPage.pagination.before_anchor 
 * });
 * 
 * === MESSAGES (chronological: [oldest → newest]) ===
 * 
 * // First page (oldest messages)
 * const firstMessagesPage = await client.listThreadMessages(threadKey, { limit: 50 });
 * 
 * // Next page (older messages)
 * const olderMessages = await client.listThreadMessages(threadKey, { 
 *   before: firstMessagesPage.pagination.after_anchor 
 * });
 * 
 * // Previous page (newer messages) 
 * const newerMessages = await client.listThreadMessages(threadKey, { 
 *   after: firstMessagesPage.pagination.before_anchor 
 * });
 * 
 * // Jump to specific position
 * const anchoredPage = await client.listThreads({ 
 *   anchor: "some_reference_id" 
 * });
 */