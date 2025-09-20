import { ApiError } from './errors';

export type HttpOptions = {
  timeoutMs?: number;
  maxRetries?: number;
};

/**
 * Sleep for `ms` milliseconds.
 * @param ms milliseconds to sleep
 * @returns promise that resolves after the delay
 */
function sleep(ms: number) {
  return new Promise((res) => setTimeout(res, ms));
}

/**
 * Perform an HTTP request with retries and timeout.
 * @template T expected response type
 * @param baseUrl base server URL
 * @param method HTTP method
 * @param path request path (prefixed with `/`)
 * @param body optional request body
 * @param headers request headers
 * @param opts timeout and retry options
 * @returns parsed response body as T
 * @throws ApiError on non-2xx responses
 */
export async function httpRequest<T>(
  baseUrl: string,
  method: string,
  path: string,
  body?: any,
  headers: Record<string,string> = {},
  opts: HttpOptions = {}
): Promise<T> {
  const url = baseUrl.replace(/\/$/, '') + path;
  const timeoutMs = opts.timeoutMs ?? 10000;
  const maxRetries = opts.maxRetries ?? 2;

  let attempt = 0;
  while (true) {
    attempt++;
    const controller = typeof AbortController !== 'undefined' ? new AbortController() : null;
    const id = controller ? setTimeout(() => controller.abort(), timeoutMs) : null;
    try {
      const res = await fetch(url, {
        method,
        headers: Object.assign({'Content-Type':'application/json'}, headers),
        body: body == null ? undefined : JSON.stringify(body),
        signal: controller ? controller.signal : undefined,
      } as any);
      if (id) clearTimeout(id);
      const text = await res.text();
      const contentType = res.headers.get('content-type') || '';
      const parsed = contentType.includes('application/json') && text ? JSON.parse(text) : text;
      if (!res.ok) throw new ApiError(res.status, parsed);
      return parsed as T;
    } catch (err) {
      if (err instanceof ApiError) throw err;
      // retry on network/timeout errors
      if (attempt > maxRetries) throw err;
      await sleep(100 * Math.pow(2, attempt));
      continue;
    }
  }
}
