import { useEffect, useState } from 'react';
import { useProgressClient } from './client';
import { validatePaginationQuery } from '../utils/validation';
import type { ThreadCreateRequestType, ThreadUpdateRequestType, PaginationResponseType, ThreadsListResponseType, ThreadResponseType, ThreadListQueryType, ThreadType, ApiErrorResponseType } from '@progressdb/js';

/**
 * Hook: list threads.
 * Threads are returned in reverse chronological order: [newest → oldest]
 * 
 * Pagination semantics for threads:
 * - before: load newer threads (scroll up) → prepend to array
 * - after: load older threads (scroll down) → append to array
 * - anchor: jump to specific position in thread list
 * - limit: number of threads to return (1-100)
 * - sort_by: sort threads by 'created_ts' or 'updated_ts'
 * 
 * @param query optional query parameters
 * @param deps optional dependency array
 */
export function useThreads(
  query: ThreadListQueryType = {}, 
  deps: any[] = []
) {
  const client = useProgressClient();
  const [threads, setThreads] = useState<ThreadType[] | null>(null);
  const [pagination, setPagination] = useState<PaginationResponseType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiErrorResponseType | null>(null);
  const [currentQuery, setCurrentQuery] = useState<ThreadListQueryType>(query);

  const fetchThreads = async (customQuery?: ThreadListQueryType) => {
    setLoading(true);
    setError(null);
    try {
      const queryToUse = customQuery || currentQuery;
      validatePaginationQuery(queryToUse);
      const res: ThreadsListResponseType = await client.listThreads(queryToUse);
      setThreads(res.threads || []);
      setPagination(res.pagination || null);
      if (customQuery) {
        setCurrentQuery(customQuery);
      }
    } catch (err) {
      setError(err as ApiErrorResponseType);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchThreads();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  const create = async (t: ThreadCreateRequestType) => {
    const res = await client.createThread(t);
    
    // For thread creation, we need to fetch the full thread since we only get the key
    const threadRes = await client.getThread(res.key);
    
    // Optimistically add to threads list if we have existing data
    if (threads && pagination) {
      // New thread should appear at the top (newest first) - PREPEND
      setThreads([threadRes.thread, ...threads]);
    } else {
      // Fallback to full refresh if no existing data
      await fetchThreads();
    }
    
    return threadRes.thread;
  };

  const update = async (threadKey: string, patch: ThreadUpdateRequestType) => {
    await client.updateThread(threadKey, patch);
    const res: ThreadResponseType = await client.getThread(threadKey);
    
    // Optimistically update threads list if we have existing data
    if (threads) {
      setThreads(threads.map(thread => 
        thread.key === threadKey ? res.thread : thread
      ));
    }
    
    return res.thread;
  };

  const remove = async (threadKey: string) => {
    await client.deleteThread(threadKey);
    await fetchThreads();
  };

  // Navigation helpers for Threads (TI): [newest → oldest] reverse chronological
  // before = newer threads, after = older threads
  const loadOlder = async () => {
    // Load older threads (scroll down)
    if (pagination?.has_after && pagination.after_anchor) {
      const res = await client.listThreads({ ...currentQuery, after: pagination.after_anchor });
      setThreads([...(threads || []), ...res.threads]); // APPEND older threads
      setPagination(res.pagination);
    }
  };

  const loadNewer = async () => {
    // Load newer threads (scroll up)
    if (pagination?.has_before && pagination.before_anchor) {
      const res = await client.listThreads({ ...currentQuery, before: pagination.before_anchor! });
      setThreads([...res.threads, ...(threads || [])]); // PREPEND newer threads
      setPagination(res.pagination);
    }
  };

  const goToAnchor = async (anchor: string) => {
    await fetchThreads({ ...currentQuery, anchor });
  };

  const reset = async () => {
    // Reset to initial query state (clear pagination)
    await fetchThreads(query);
  };

  return { 
    threads, 
    pagination, 
    loading, 
    error, 
    refresh: fetchThreads, 
    reset,
    create, 
    update, 
    remove,
    // Navigation helpers
    loadOlder,
    loadNewer,
    goToAnchor
  };
}