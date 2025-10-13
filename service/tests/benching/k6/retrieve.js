import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';
import { options as k6Options } from './options.js';
import { buildHeaders } from './lib/headers.js';

export let options = k6Options;

const DEFAULT_TARGET = 'http://192.168.0.132:8080';
// Use thread-scoped messages endpoint
const DEFAULT_PATH = '/v1/threads';
const DEFAULT_THREAD = 't1';
const DEFAULT_LIMIT = 10;

export function setup() {
  // prefer a precomputed signature exported by the runner
  const pre = __ENV.PRECOMPUTED_SIGNATURE || '';
  const userId = __ENV.USER_ID || 'bench-user';
  if (pre) {
    return { signature: pre, userId };
  }
  const backendKey = __ENV.BACKEND_API_KEY || '';
  if (!backendKey) {
    throw new Error('BACKEND_API_KEY must be provided in env for setup signing');
  }
  const signUrl = `${__ENV.TARGET_URL || DEFAULT_TARGET}/v1/_sign`;
  const res = http.post(signUrl, JSON.stringify({ userId }), {
    headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${backendKey}` },
  });
  if (res.status < 200 || res.status >= 300) throw new Error(`sign failed: ${res.status}`);
  const j = res.json();
  return { signature: j.signature, userId };
}

const TARGET = __ENV.TARGET_URL || DEFAULT_TARGET;
const PATH = __ENV.RETRIEVE_PATH || DEFAULT_PATH;
const THREAD = __ENV.THREAD_ID || DEFAULT_THREAD;
const LIMIT = Number(__ENV.LIMIT || DEFAULT_LIMIT);

const reqTime = new Trend('req_duration_ms');
const successRate = new Rate('retrieve_success');
const failRate = new Rate('retrieve_fail');
const bytesReceived = new Counter('bytes_received_total');
const statusCodes = new Counter('status_codes');

export default function (data) {
// thread-scoped messages endpoint: /v1/threads/{threadID}/messages
const url = `${TARGET}${PATH}/${THREAD}/messages?limit=${LIMIT}`;
  const headers = buildHeaders();
  if (data.userId) headers['X-User-ID'] = data.userId;
  if (data.signature) headers['X-User-Signature'] = data.signature;
  if (__ENV.FRONTEND_API_KEY) headers['X-API-Key'] = __ENV.FRONTEND_API_KEY;

  const t0 = Date.now();
  const res = http.get(url, { headers });
  const dt = Date.now() - t0;
  reqTime.add(dt);
  bytesReceived.add(res.body ? res.body.length : 0);

  const ok = check(res, { 'status is 200': (r) => r.status === 200 });
  successRate.add(ok);
  failRate.add(!ok);
  // record status code metric with tag (k6 accepts tags as direct fields)
  statusCodes.add(1, { status: String(res.status) });
  if (__ENV.LOG_STATUSES === '1') {
    console.log(`retrieve: status=${res.status} body_len=${res.body ? res.body.length : 0}`);
  }
  // Log response bodies only for non-success statuses (>=400)
  if (res.status >= 400) {
    const max = Number(__ENV.LOG_TRUNCATE || 2000);
    try {
      const bodyStr = res.body ? String(res.body) : '';
      const out = bodyStr.length > max ? bodyStr.slice(0, max) + '...<truncated>' : bodyStr;
      console.log(JSON.stringify({ op: 'retrieve', status: res.status, body: out }));
    } catch (e) {
      console.log(`retrieve: status=${res.status} (failed to stringify body)`);
    }
  }

  sleep(1);
}
