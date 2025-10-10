import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';
import { options as k6Options } from './options.js';
import { buildHeaders } from './lib/headers.js';

export let options = k6Options;

const DEFAULT_TARGET = 'http://192.168.0.132:8080';
const TARGET = __ENV.TARGET_URL || DEFAULT_TARGET;
const TITLE_PREFIX = __ENV.THREAD_TITLE_PREFIX || 'bench-thread';

let STATIC_METADATA = null;
if (__ENV.THREAD_METADATA) {
  try {
    STATIC_METADATA = JSON.parse(__ENV.THREAD_METADATA);
  } catch (err) {
    console.error(`create-thread: failed to parse THREAD_METADATA env, ignoring. err=${err}`);
  }
}

const reqTime = new Trend('thread_create_req_duration_ms');
const successRate = new Rate('thread_create_success');
const failRate = new Rate('thread_create_fail');
const bytesSent = new Counter('thread_create_bytes_sent_total');
const statusCodes = new Counter('thread_create_status_codes');

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
  const iterationId = `${__VU}-${__ITER}-${Date.now()}`;
  const title = `${TITLE_PREFIX}-${iterationId}`;

  const thread = { title };
  if (STATIC_METADATA) thread.metadata = STATIC_METADATA;
  if (data.userId) thread.author = data.userId;

  const body = JSON.stringify(thread);
  const headers = buildHeaders({ 'Content-Type': 'application/json' });
  if (data.userId) headers['X-User-ID'] = data.userId;
  if (data.signature) headers['X-User-Signature'] = data.signature;
  if (data.apiKey) {
    headers['Authorization'] = `Bearer ${data.apiKey}`;
  } else if (__ENV.FRONTEND_API_KEY) {
    headers['Authorization'] = `Bearer ${__ENV.FRONTEND_API_KEY}`;
  }

  const t0 = Date.now();
  const res = http.post(`${TARGET}/v1/threads`, body, { headers });
  const dt = Date.now() - t0;

  reqTime.add(dt);
  bytesSent.add(body.length);

  const ok = check(res, {
    'status is 200/202': (r) => r.status === 200 || r.status === 202,
  });
  successRate.add(ok);
  failRate.add(!ok);
  statusCodes.add(1, { status: String(res.status) });

  if (!ok) {
    try {
      const json = res.json();
      const message = json && (json.reason || json.message || JSON.stringify(json));
      console.log(`create-thread: failed status=${res.status} message=${message}`);
    } catch (err) {
      console.log(`create-thread: failed status=${res.status} body=${res.body}`);
    }
  }

  sleep(1);
}

