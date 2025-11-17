import { useEffect, useState } from 'react';
import { useProgressClient } from './client';
import type { ThreadCreateRequest, ThreadUpdateRequest, PaginationResponse, ThreadsListResponse, ThreadResponse } from '@progressdb/js';

/**
 * Hook: list threads.
 * Threads are returned in reverse chronological order: [newest → oldest]
 * 
 * Pagination semantics for threads:
 * - before: load newer threads (scroll up) → prepend to array
 * - after: load older threads (scroll down) → append to array
 * 
 * @param query optional query parameters
 * @param deps optional dependency array
 */
export function useThreads(
  query: { title?: string; slug?: string; limit?: number; before?: string; after?: string; anchor?: string; sort_by?: 'created_ts' | 'updated_ts'; author?: string } = {}, 
  deps: any[] = []
) {
  const client = useProgressClient();
  const [threads, setThreads] = useState<any[] | null>(null);
  const [pagination, setPagination] = useState<PaginationResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);
  const [currentQuery, setCurrentQuery] = useState(query);

  const fetchThreads = async (customQuery?: typeof query) => {
    setLoading(true);
    setError(null);
    try {
      const queryToUse = customQuery || currentQuery;
      const res: ThreadsListResponse = await client.listThreads(queryToUse);
      setThreads(res.threads || []);
      setPagination(res.pagination || null);
      if (customQuery) {
        setCurrentQuery(customQuery);
      }
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchThreads();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  const create = async (t: ThreadCreateRequest) => {
    const res = await client.createThread(t);
    await fetchThreads();
    return res;
  };

  const update = async (threadKey: string, patch: ThreadUpdateRequest) => {
    await client.updateThread(threadKey, patch);
    await fetchThreads();
    const res: ThreadResponse = await client.getThread(threadKey);
    return res.thread;
  };

  const remove = async (threadKey: string) => {
    await client.deleteThread(threadKey);
    await fetchThreads();
  };

  // Navigation helpers for Threads (TI): [newest → oldest] reverse chronological
  // before = newer threads, after = older threads
  const nextPage = async () => {
    // Next page = older threads (scroll down)
    if (pagination?.has_after && pagination.after_anchor) {
      await fetchThreads({ ...currentQuery, after: pagination.after_anchor });
    }
  };

  const prevPage = async () => {
    // Previous page = newer threads (scroll up)
    if (pagination?.has_before && pagination.before_anchor) {
      await fetchThreads({ ...currentQuery, before: pagination.before_anchor });
    }
  };

  const goToAnchor = async (anchor: string) => {
    await fetchThreads({ ...currentQuery, anchor });
  };

  const loadMore = async () => {
    // Load more = older threads (infinite scroll down)
    if (pagination?.has_after && pagination.after_anchor) {
      setLoading(true);
      try {
        const queryToUse = { ...currentQuery, after: pagination.after_anchor };
        const res: ThreadsListResponse = await client.listThreads(queryToUse);
        setThreads(prev => [...(prev || []), ...(res.threads || [])]);
        setPagination(res.pagination || null);
        setCurrentQuery(queryToUse);
      } catch (err) {
        setError(err);
      } finally {
        setLoading(false);
      }
    }
  };

  return { 
    threads, 
    pagination, 
    loading, 
    error, 
    refresh: fetchThreads, 
    create, 
    update, 
    remove,
    // Navigation helpers
    nextPage,
    prevPage,
    goToAnchor,
    loadMore
  };
}