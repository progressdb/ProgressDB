import type { SDKOptionsType } from '../types';

/**
 * Build request headers used by frontend SDK.
 * @param apiKey frontend API key to send as `X-API-Key`
 * @param userId optional user id to send as X-User-ID
 * @param userSignature optional signature to send as X-User-Signature
 * @param hasBody whether the request has a body (affects Content-Type header)
 * @returns headers object
 */
export function buildHeaders(apiKey?: string, userId?: string, userSignature?: string, hasBody: boolean = true) {
  const headers: Record<string, string> = {};
  if (apiKey) headers['X-API-Key'] = apiKey;
  if (userId) headers['X-User-ID'] = userId;
  if (userSignature) headers['X-User-Signature'] = userSignature;
  if (hasBody) headers['Content-Type'] = 'application/json';
  return headers;
}