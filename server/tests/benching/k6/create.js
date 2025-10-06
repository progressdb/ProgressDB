import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';
import { generatePayload } from './lib/payload.js';
import { buildHeaders } from './lib/headers.js';
import { options as k6Options } from './options.js';

// export k6 options
export let options = k6Options;

// target url and payload size from environment or defaults
const target = __ENV.TARGET_URL || "http://192.168.0.132:8080";
const size = Number(__ENV.PAYLOAD_SIZE || 500000);

// setup runs once before the test
export function setup() {
  // get signature, user id, and api key from environment or use defaults
  const signature = __ENV.GENERATED_USER_SIGNATURE;
  const userId = __ENV.USER_ID;
  const apiKey = __ENV.FRONTEND_API_KEY;
  console.log('setup: signature:', signature);
  console.log('setup: userId:', userId);
  console.log('setup: apiKey:', apiKey);
  return { signature, userId, apiKey };
}

// define metrics
const reqTime = new Trend('req_duration_ms');
const successRate = new Rate('create_success');
const failRate = new Rate('create_fail');
const bytesSent = new Counter('bytes_sent_total');
const statusCodes = new Counter('status_codes');

// main function for each virtual user iteration
export default function (data) {
  // generate payload and checksum
  const { payload, checksum } = generatePayload(size);

  // build request url and body
  const url = `${target}/v1/messages`;
  const bodyObj = { body: payload, checksum };
  if (data.userId) bodyObj.author = data.userId;
  const body = JSON.stringify(bodyObj);

  // build headers, override with data if present
  const headers = buildHeaders();

  // record request start time
  const t0 = Date.now();

  // send post request
  const res = http.post(url, body, { headers });

  // calculate request duration
  const dt = Date.now() - t0;
  reqTime.add(dt);
  bytesSent.add(body.length);

  // check for success (status 200 or 201)
  const ok = check(res, { 'status is 200 or 201': (r) => r.status === 200 || r.status === 201 });
  successRate.add(ok);
  failRate.add(!ok);

  // record status code metric
  statusCodes.add(1, { status: String(res.status) });

  // log response body and rejection reason/message for errors (status >= 400)
  if (res.status >= 400) {
    let msg = '';
    try {
      const body = res.json();
      msg = body && (body.reason || body.message || JSON.stringify(body));
    } catch (e) {
      msg = res.body;
    }
    console.log(`create: response status code = ${res.status}, message: ${msg}`);
  }

  // sleep for pacing
  sleep(1);
}
