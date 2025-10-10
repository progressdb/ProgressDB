import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';
import { options as k6Options } from './options.js';
import { buildHeaders } from './lib/headers.js';

export let options = k6Options;

const DEFAULT_TARGET = 'http://192.168.0.132:8080';
const TARGET = __ENV.TARGET_URL || DEFAULT_TARGET;
const PATH = __ENV.THREADS_PATH || '/v1/threads';
const AUTHOR_OVERRIDE = __ENV.THREADS_AUTHOR || '';

const reqTime = new Trend('thread_list_req_duration_ms');
const successRate = new Rate('thread_list_success');
const failRate = new Rate('thread_list_fail');
const bytesReceived = new Counter('thread_list_bytes_received_total');
const statusCodes = new Counter('thread_list_status_codes');

export function setup() {
  const userId = __ENV.USER_ID || 'bench-user';
  const apiKey = __ENV.FRONTEND_API_KEY || '';
  const pre = __ENV.GENERATED_USER_SIGNATURE || __ENV.PRECOMPUTED_SIGNATURE || '';
  if (pre) {
    console.log(`setup: using provided signature for user=${userId}`);
    return { signature: pre, userId, apiKey };
  }

  const backendKey = __ENV.BACKEND_API_KEY || '';
  if (!backendKey) {
    console.warn('setup: no signature provided and BACKEND_API_KEY missing; proceeding without signature');
    return { signature: '', userId, apiKey };
  }

  const signUrl = `${TARGET}/v1/_sign`;
  const payload = JSON.stringify({ userId });
  const res = http.post(signUrl, payload, {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${backendKey}`,
    },
  });
  if (res.status < 200 || res.status >= 300) {
    throw new Error(`setup: signature request failed status=${res.status}`);
  }
  const body = res.json();
  const signature = body && body.signature ? body.signature : '';
  if (!signature) {
    throw new Error('setup: signature response missing signature field');
  }
  console.log(`setup: fetched signature for user=${userId}`);
  return { signature, userId, apiKey };
}

export default function (data) {
  const headers = buildHeaders();
  if (data.userId) headers['X-User-ID'] = data.userId;
  if (data.signature) headers['X-User-Signature'] = data.signature;
  if (data.apiKey) {
    headers['Authorization'] = `Bearer ${data.apiKey}`;
  } else if (__ENV.FRONTEND_API_KEY) {
    headers['Authorization'] = `Bearer ${__ENV.FRONTEND_API_KEY}`;
  }

  let url = `${TARGET}${PATH}`;
  const author = AUTHOR_OVERRIDE || data.userId || __ENV.USER_ID || '';
  if (author) {
    const sep = url.includes('?') ? '&' : '?';
    url = `${url}${sep}author=${encodeURIComponent(author)}`;
  }

  const t0 = Date.now();
  const res = http.get(url, { headers });
  const dt = Date.now() - t0;
  reqTime.add(dt);
  bytesReceived.add(res.body ? res.body.length : 0);

  const ok = check(res, {
    'status is 200': (r) => r.status === 200,
  });
  successRate.add(ok);
  failRate.add(!ok);
  statusCodes.add(1, { status: String(res.status) });

  if (!ok) {
    const truncate = Number(__ENV.LOG_TRUNCATE || 2000);
    try {
      const str = res.body ? String(res.body) : '';
      const out = str.length > truncate ? `${str.slice(0, truncate)}...<truncated>` : str;
      console.log(`get-threads: failed status=${res.status} body=${out}`);
    } catch (err) {
      console.log(`get-threads: failed status=${res.status} (unable to stringify body)`);
    }
  }

  sleep(1);
}

