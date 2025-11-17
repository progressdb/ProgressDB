import { useContext } from 'react';
import { ProgressClientContext } from '../provider/context';

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