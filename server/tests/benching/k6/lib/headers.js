// Builds request headers using environment variables and optional overrides.
export function buildHeaders(extra = {}) {
  const headers = { ...extra };

  const userSig = __ENV.GENERATED_USER_SIGNATURE;
  const userID = __ENV.USER_ID;
  const apiKey = __ENV.FRONTEND_API_KEY;

  if (apiKey) headers['X-API-Key'] = apiKey;
  if (userID) headers['X-User-ID'] = userID;
  if (userSig) headers['X-User-Signature'] = userSig;

  headers['X-Requested-With'] = 'k6-bench';

  return headers;
}
