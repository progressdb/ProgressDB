import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';

// Import the underlying ProgressDBClient and types from the TS SDK
// Import the published JS SDK package instead of referencing local TS sources.
// This ensures the react package depends on the compiled @progressdb/js package.
// Use the local TypeScript SDK types so the React package and TS SDK
// share the same type definitions during development. This ensures
// `removeReaction` and other APIs have consistent signatures.
import ProgressDBClient, { SDKOptions, Message, Thread, ReactionInput } from '@progressdb/js';

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
 * @param threadId thread id to list messages for
 * @param deps optional dependency array to re-run fetch
 */
export function useMessages(threadId?: string, deps: any[] = []) {
  const client = useProgressClient();
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchMessages = async () => {
    if (!threadId) return;
    setLoading(true);
    setError(null);
    try {
      const res = await client.listMessages({ thread: threadId });
      setMessages(res.messages || []);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (threadId) fetchMessages();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId, ...deps]);

  const create = async (msg: Message) => {
    const created = await client.createThreadMessage(threadId || '', msg);
    // naive refresh
    await fetchMessages();
    return created;
  };

  return { messages, loading, error, refresh: fetchMessages, create };
}

// Hook for a single message
/**
 * Hook: fetch/operate on a single message within a thread.
 * @param threadId id of the thread containing the message
 * @param id message id
 */
export function useMessage(threadId?: string, id?: string) {
  const client = useProgressClient();
  const [message, setMessage] = useState<Message | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchMessage = async () => {
    if (!id || !threadId) return;
    setLoading(true);
    setError(null);
    try {
      const m = await client.getThreadMessage(threadId, id);
      setMessage(m);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (id && threadId) fetchMessage();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, threadId]);

  const update = async (msg: Message) => {
    if (!id || !threadId) throw new Error('threadId and id required');
    const updated = await client.updateThreadMessage(threadId, id, msg);
    setMessage(updated);
    return updated;
  };

  const remove = async () => {
    if (!id || !threadId) throw new Error('threadId and id required');
    await client.deleteThreadMessage(threadId, id);
    setMessage(null);
  };

  return { message, loading, error, refresh: fetchMessage, update, remove };
}

// Simple thread hooks
/**
 * Hook: list threads.
 * @param deps optional dependency array
 */
export function useThreads(deps: any[] = []) {
  const client = useProgressClient();
  const [threads, setThreads] = useState<Thread[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchThreads = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await client.listThreads();
      setThreads(res.threads || []);
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

  const create = async (t: Partial<Thread>) => {
    const created = await client.createThread(t as Thread);
    await fetchThreads();
    return created;
  };

  const update = async (id: string, patch: Partial<Thread>) => {
    const updated = await client.updateThread(id, patch as Thread);
    await fetchThreads();
    return updated;
  };

  const remove = async (id: string) => {
    await client.deleteThread(id);
    await fetchThreads();
  };

  return { threads, loading, error, refresh: fetchThreads, create, update, remove };
}

// Reactions
/**
 * Hook: list/add/remove reactions for a message in a thread.
 * @param threadId thread id
 * @param messageId message id
 */
export function useReactions(threadId?: string, messageId?: string) {
  const client = useProgressClient();
  const [reactions, setReactions] = useState<Array<{ id: string; reaction: string }> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchReactions = async () => {
    if (!messageId || !threadId) return;
    setLoading(true);
    setError(null);
    try {
      const res = await client.listReactions(threadId, messageId);
      setReactions(res.reactions || []);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (messageId && threadId) fetchReactions();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messageId, threadId]);

  const add = async (input: ReactionInput) => {
    if (!messageId || !threadId) throw new Error('threadId and messageId required');
    const res = await client.addOrUpdateReaction(threadId, messageId, input);
    await fetchReactions();
    return res;
  };

  const remove = async (identity: string) => {
    if (!messageId || !threadId) throw new Error('threadId and messageId required');
    await client.removeReaction(threadId, messageId, identity);
    await fetchReactions();
  };

  return { reactions, loading, error, refresh: fetchReactions, add, remove };
}

export type { Message, Thread };

export default ProgressDBProvider;
