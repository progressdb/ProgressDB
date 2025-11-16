import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';

// Import ProgressDBClient and types from local TypeScript SDK during development.
// In production, import from the published @progressdb/js package for compatibility.
// This approach ensures consistent type definitions across both the React and TS SDKs.
// To switch between local and published SDKs, update the import path as needed.
import ProgressDBClient, { SDKOptions, Message, Thread, ThreadsListResponse, MessagesListResponse, ThreadResponse, MessageResponse, PaginationRequest, PaginationResponse, ThreadCreateRequest, ThreadUpdateRequest, MessageCreateRequest, MessageUpdateRequest } from '../../../typescript/src/index';

// Provider + context
export type UserSignature = { userId: string; signature: string };
export type GetUserSignature = () => Promise<UserSignature> | UserSignature;

export type ProgressProviderProps = {
  children: React.ReactNode;
  options?: SDKOptions;
  /**
   * REQUIRED function used to obtain a `{ userId, signature }` pair for the current user.
   * The provider calls this function (can be async) once and attaches the returned values to
   * the underlying SDK as `defaultUserId` and `defaultUserSignature`.
   */
  getUserSignature: GetUserSignature;
  /**
   * Persist signature in `sessionStorage` to survive navigation/re-renders in the same tab.
   * Default: true
   */
  persistSignature?: boolean;
};

type ProgressClientContextValue = {
  client: ProgressDBClient;
  userId?: string;
  signature?: string;
  signatureLoaded: boolean;
  signatureLoading: boolean;
  signatureError?: any;
  refreshUserSignature: () => Promise<void>;
  clearUserSignature: () => void;
};

const ProgressClientContext = createContext<ProgressClientContextValue | null>(null);

/**
 * ProgressDBProvider wraps the React app and provides a configured
 * `ProgressDBClient` instance plus the current user's signature.
 *
 * The provider calls `getUserSignature` (can be async) to obtain a
 * `{ userId, signature }` pair and attaches them to the underlying
 * SDK as `defaultUserId` and `defaultUserSignature`.
 */
export const ProgressDBProvider: React.FC<ProgressProviderProps> = ({ children, options, getUserSignature, persistSignature = true }) => {
  const client = useMemo(() => new ProgressDBClient(options || {}), [JSON.stringify(options || {})]);

  const storageKey = useMemo(() => {
    const base = (options && options.baseUrl) || '';
    return `progressdb:signature:${base}`;
  }, [JSON.stringify(options || {})]);

  const [signatureLoaded, setSignatureLoaded] = useState(false);
  const [signatureLoading, setSignatureLoading] = useState(false);
  const [signatureError, setSignatureError] = useState<any>(undefined);
  const [userId, setUserId] = useState<string | undefined>(undefined);
  const [signature, setSignature] = useState<string | undefined>(undefined);

  const applySignature = (s?: UserSignature) => {
    if (!s) return;
    client.defaultUserId = s.userId;
    client.defaultUserSignature = s.signature;
    setUserId(s.userId);
    setSignature(s.signature);
  };

  const refreshUserSignature = async () => {
    setSignatureLoading(true);
    setSignatureError(undefined);
    try {
      const res = await getUserSignature();
      applySignature(res);
      if (persistSignature && typeof sessionStorage !== 'undefined') {
        sessionStorage.setItem(storageKey, JSON.stringify(res));
      }
      setSignatureLoaded(true);
    } catch (err) {
      setSignatureError(err);
      // eslint-disable-next-line no-console
      console.error('ProgressDB getUserSignature failed', err);
    } finally {
      setSignatureLoading(false);
    }
  };

  const clearUserSignature = () => {
    client.defaultUserId = undefined;
    client.defaultUserSignature = undefined;
    setUserId(undefined);
    setSignature(undefined);
    setSignatureLoaded(false);
    setSignatureError(undefined);
    if (persistSignature && typeof sessionStorage !== 'undefined') {
      sessionStorage.removeItem(storageKey);
    }
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setSignatureLoading(true);
      try {
        if (persistSignature && typeof sessionStorage !== 'undefined') {
          const raw = sessionStorage.getItem(storageKey);
          if (raw) {
            try {
              const parsed: UserSignature = JSON.parse(raw);
              if (!cancelled) {
                applySignature(parsed);
                setSignatureLoaded(true);
                setSignatureLoading(false);
                return;
              }
            } catch (e) {
              // ignore parse errors
            }
          }
        }

        // fall back to calling getUserSignature
        const res = await getUserSignature();
        if (cancelled) return;
        applySignature(res);
        if (persistSignature && typeof sessionStorage !== 'undefined') {
          sessionStorage.setItem(storageKey, JSON.stringify(res));
        }
        setSignatureLoaded(true);
      } catch (err) {
        setSignatureError(err);
        // eslint-disable-next-line no-console
        console.error('ProgressDB getUserSignature failed', err);
      } finally {
        if (!cancelled) setSignatureLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [getUserSignature, storageKey]);

  const ctxVal: ProgressClientContextValue = useMemo(() => ({
    client,
    userId,
    signature,
    signatureLoaded,
    signatureLoading,
    signatureError,
    refreshUserSignature,
    clearUserSignature,
  }), [client, userId, signature, signatureLoaded, signatureLoading, signatureError]);

  return <ProgressClientContext.Provider value={ctxVal}>{children}</ProgressClientContext.Provider>;
};

/**
 * Hook: get the underlying `ProgressDBClient` from context.
 * @throws if used outside a `ProgressDBProvider`
 */
export function useProgressClient() {
  const ctx = useContext(ProgressClientContext);
  if (!ctx) throw new Error('useProgressClient must be used within ProgressDBProvider');
  return ctx.client;
}

/**
 * Hook: read the current user signature from context.
 * Returns `{ userId, signature, loaded, loading, error, refresh, clear }`.
 * @throws if used outside a `ProgressDBProvider`
 */
export function useUserSignature() {
  const ctx = useContext(ProgressClientContext);
  if (!ctx) throw new Error('useUserSignature must be used within ProgressDBProvider');
  return {
    userId: ctx.userId,
    signature: ctx.signature,
    loaded: ctx.signatureLoaded,
    loading: ctx.signatureLoading,
    error: ctx.signatureError,
    refresh: ctx.refreshUserSignature,
    clear: ctx.clearUserSignature,
  };
}

// Basic hook: list messages for a thread
/**
 * Hook: list messages for a given thread.
 * Messages are returned in chronological order: [oldest → newest]
 * 
 * Pagination semantics for messages:
 * - before: load older messages (scroll up) → append to array
 * - after: load newer messages (scroll down) → append to array
 * 
 * @param threadKey thread key to list messages for
 * @param query optional pagination query parameters (limit, before, after, anchor, sort_by)
 * @param deps optional dependency array to re-run fetch
 */
export function useMessages(threadKey?: string, query: { limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string } = {}, deps: any[] = []) {
  const client = useProgressClient();
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [pagination, setPagination] = useState<PaginationResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);
  const [currentQuery, setCurrentQuery] = useState(query);

  const fetchMessages = async (customQuery?: typeof query) => {
    if (!threadKey) return;
    setLoading(true);
    setError(null);
    try {
      const queryToUse = customQuery || currentQuery;
      const res: MessagesListResponse = await client.listThreadMessages(threadKey, queryToUse);
      setMessages(res.messages || []);
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
    if (threadKey) fetchMessages();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadKey, ...deps]);

  const create = async (msg: MessageCreateRequest) => {
    const created: MessageResponse = await client.createThreadMessage(threadKey || '', msg);
    // naive refresh
    await fetchMessages();
    return created.message;
  };

  // Navigation helpers for Messages (MI): [oldest → newest] chronological
  // before = older messages, after = newer messages
  const nextPage = async () => {
    // Next page = older messages (scroll up)
    if (pagination?.has_before && pagination.before_anchor) {
      await fetchMessages({ ...currentQuery, before: pagination.before_anchor });
    }
  };

  const prevPage = async () => {
    // Previous page = newer messages (scroll down)
    if (pagination?.has_after && pagination.after_anchor) {
      await fetchMessages({ ...currentQuery, after: pagination.after_anchor });
    }
  };

  const goToAnchor = async (anchor: string) => {
    await fetchMessages({ ...currentQuery, anchor });
  };

  const loadMore = async () => {
    // Load more = older messages (infinite scroll up)
    if (pagination?.has_before && pagination.before_anchor) {
      setLoading(true);
      try {
        const queryToUse = { ...currentQuery, before: pagination.before_anchor };
        const res: MessagesListResponse = await client.listThreadMessages(threadKey, queryToUse);
        setMessages(prev => [...(prev || []), ...(res.messages || [])]);
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
    messages, 
    pagination, 
    loading, 
    error, 
    refresh: fetchMessages, 
    create,
    // Navigation helpers
    nextPage,
    prevPage,
    goToAnchor,
    loadMore
  };
}

// Hook for a single message
/**
 * Hook: fetch/operate on a single message within a thread.
 * @param threadKey key of the thread containing the message
 * @param id message id
 */
export function useMessage(threadKey?: string, id?: string) {
  const client = useProgressClient();
  const [message, setMessage] = useState<Message | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchMessage = async () => {
    if (!id || !threadKey) return;
    setLoading(true);
    setError(null);
    try {
      const res: MessageResponse = await client.getThreadMessage(threadKey, id);
      setMessage(res.message);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (id && threadKey) fetchMessage();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, threadKey]);

  const update = async (msg: MessageUpdateRequest) => {
    if (!id || !threadKey) throw new Error('threadKey and id required');
    const res: MessageResponse = await client.updateThreadMessage(threadKey, id, msg);
    setMessage(res.message);
    return res.message;
  };

  const remove = async () => {
    if (!id || !threadKey) throw new Error('threadKey and id required');
    await client.deleteThreadMessage(threadKey, id);
    setMessage(null);
  };

  return { message, loading, error, refresh: fetchMessage, update, remove };
}

// Simple thread hooks
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
export function useThreads(query: { title?: string; slug?: string; limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string; author?: string } = {}, deps: any[] = []) {
  const client = useProgressClient();
  const [threads, setThreads] = useState<Thread[] | null>(null);
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
    const res: ThreadResponse = await client.updateThread(threadKey, patch);
    await fetchThreads();
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

// Health check hooks
/**
 * Hook: basic health check.
 */
export function useHealthz() {
  const client = useProgressClient();
  const [data, setData] = useState<{ status: string } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await client.healthz();
      setData(result);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetch();
  }, []);

  return { data, loading, error, refresh: fetch };
}

/**
 * Hook: readiness check with version info.
 */
export function useReadyz() {
  const client = useProgressClient();
  const [data, setData] = useState<{ status: string; version?: string } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await client.readyz();
      setData(result);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetch();
  }, []);

  return { data, loading, error, refresh: fetch };
}

/**
 * Pagination Usage Examples:
 * 
 * // Messages (chronological): [oldest → newest]
 * const { messages, nextPage, prevPage, loadMore } = useMessages(threadKey);
 * // nextPage() loads older messages (scroll up)
 * // prevPage() loads newer messages (scroll down)
 * // loadMore() loads older messages (infinite scroll)
 * 
 * // Threads (reverse chronological): [newest → oldest]  
 * const { threads, nextPage, prevPage, loadMore } = useThreads();
 * // nextPage() loads older threads (scroll down)
 * // prevPage() loads newer threads (scroll up)
 * // loadMore() loads older threads (infinite scroll)
 */

export type { Message, Thread, ThreadCreateRequest, ThreadUpdateRequest, MessageCreateRequest, MessageUpdateRequest };

export default ProgressDBProvider;
