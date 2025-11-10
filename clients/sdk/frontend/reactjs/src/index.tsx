import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';

// Import the underlying ProgressDBClient and types from the TS SDK
// Import the published JS SDK package instead of referencing local TS sources.
// This ensures the react package depends on the compiled @progressdb/js package.
// Use the local TypeScript SDK types so the React package and TS SDK
// share the same type definitions during development. This ensures
// `removeReaction` and other APIs have consistent signatures.
// Import from local TypeScript SDK during development
// In production, this should import from @progressdb/js
// Import from local TypeScript SDK during development
// In production, this should import from @progressdb/js
import ProgressDBClient, { SDKOptions, Message, Thread, ThreadsListResponse, MessagesListResponse, ThreadResponse, MessageResponse, PaginationRequest, PaginationResponse } from '../../../typescript/src/index';

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
 * @param threadKey thread key to list messages for
 * @param query optional pagination query parameters
 * @param deps optional dependency array to re-run fetch
 */
export function useMessages(threadKey?: string, query: { limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string; include_deleted?: boolean } = {}, deps: any[] = []) {
  const client = useProgressClient();
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [pagination, setPagination] = useState<PaginationResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchMessages = async () => {
    if (!threadKey) return;
    setLoading(true);
    setError(null);
    try {
      const res: MessagesListResponse = await client.listThreadMessages(threadKey, query);
      setMessages(res.messages || []);
      setPagination(res.pagination || null);
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

  const create = async (msg: Message) => {
    const created: MessageResponse = await client.createThreadMessage(threadKey || '', msg);
    // naive refresh
    await fetchMessages();
    return created.message;
  };

  return { messages, pagination, loading, error, refresh: fetchMessages, create };
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

  const update = async (msg: Message) => {
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
 * @param query optional query parameters
 * @param deps optional dependency array
 */
export function useThreads(query: { title?: string; slug?: string; limit?: number; before?: string; after?: string; anchor?: string; sort_by?: string; author?: string } = {}, deps: any[] = []) {
  const client = useProgressClient();
  const [threads, setThreads] = useState<Thread[] | null>(null);
  const [pagination, setPagination] = useState<PaginationResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchThreads = async () => {
    setLoading(true);
    setError(null);
    try {
      const res: ThreadsListResponse = await client.listThreads(query);
      setThreads(res.threads || []);
      setPagination(res.pagination || null);
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

  const create = async (t: { title: string; slug?: string }) => {
    const res: ThreadResponse = await client.createThread(t);
    await fetchThreads();
    return res.thread;
  };

  const update = async (threadKey: string, patch: { title?: string; slug?: string }) => {
    const res: ThreadResponse = await client.updateThread(threadKey, patch);
    await fetchThreads();
    return res.thread;
  };

  const remove = async (threadKey: string) => {
    await client.deleteThread(threadKey);
    await fetchThreads();
  };

  return { threads, pagination, loading, error, refresh: fetchThreads, create, update, remove };
}

// Reactions are not in the OpenAPI spec - removing this hook
// If reactions are needed, they should be added to the OpenAPI spec first

export type { Message, Thread };

export default ProgressDBProvider;
