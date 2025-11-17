export type SDKOptionsType = {
  baseUrl?: string;
  apiKey?: string; // frontend API key sent as X-API-Key
  defaultUserId?: string;
  defaultUserSignature?: string;
  fetch?: typeof fetch;
};