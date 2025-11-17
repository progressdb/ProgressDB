import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';
import ProgressDBClient from '@progressdb/js';
import { ProgressClientContext } from './context';
import type { UserSignature, ProgressProviderProps } from '../types/provider';

/**
 * ProgressDBProvider wraps the React app and provides a configured
 * `ProgressDBClient` instance plus the current user's signature.
 *
 * The provider calls `getUserSignature` (can be async) to obtain a
 * `{ userId, signature }` pair and attaches them to the underlying
 * SDK as `defaultUserId` and `defaultUserSignature`.
 */
export const ProgressDBProvider: React.FC<ProgressProviderProps> = ({ 
  children, 
  options, 
  getUserSignature, 
  persistSignature = true 
}) => {
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

  const ctxVal = useMemo(() => ({
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

export default ProgressDBProvider;