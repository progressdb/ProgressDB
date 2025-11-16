import type { SDKOptions } from '../types';

/**
 * Build request headers used by the frontend SDK.
 * @param apiKey frontend API key to send as `X-API-Key`
 * @param userId optional user id to send as `X-User-ID`
 * @param userSignature optional signature to send as `X-User-Signature`
 * @returns headers object
 */
export function buildHeaders(apiKey?: string, userId?: string, userSignature?: string) {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  };
  if (apiKey) headers['X-API-Key'] = apiKey;
  if (userId) headers['X-User-ID'] = userId;
  if (userSignature) headers['X-User-Signature'] = userSignature;
  return headers;
}