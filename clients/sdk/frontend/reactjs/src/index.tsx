import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';

// Import the underlying ProgressDBClient and types from the TS SDK
import ProgressDBClient, { SDKOptions, Message, Thread } from '../../typescript/src/index';

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

export function useProgressClient() {
  const ctx = useContext(ProgressClientContext);
  if (!ctx) throw new Error('useProgressClient must be used within ProgressDBProvider');
  return ctx.client;
}

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
export function useMessage(id?: string) {
  const client = useProgressClient();
  const [message, setMessage] = useState<Message | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchMessage = async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const m = await client.getMessage(id);
      setMessage(m);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (id) fetchMessage();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const update = async (msg: Message) => {
    const updated = await client.updateMessage(id || '', msg);
    setMessage(updated);
    return updated;
  };

  const remove = async () => {
    await client.deleteMessage(id || '');
    setMessage(null);
  };

  return { message, loading, error, refresh: fetchMessage, update, remove };
}

// Simple thread hooks
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

  return { threads, loading, error, refresh: fetchThreads, create };
}

// Reactions
export function useReactions(messageId?: string) {
  const client = useProgressClient();
  const [reactions, setReactions] = useState<Array<{ id: string; reaction: string }> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchReactions = async () => {
    if (!messageId) return;
    setLoading(true);
    setError(null);
    try {
      const res = await client.listReactions(messageId);
      setReactions(res.reactions || []);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (messageId) fetchReactions();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messageId]);

  const add = async (input: { id: string; reaction: string }) => {
    const res = await client.addOrUpdateReaction(messageId || '', input);
    await fetchReactions();
    return res;
  };

  const remove = async (identity: string) => {
    await client.removeReaction(messageId || '', identity);
    await fetchReactions();
  };

  return { reactions, loading, error, refresh: fetchReactions, add, remove };
}

export type { Message, Thread };

export default ProgressDBProvider;
