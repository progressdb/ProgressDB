import http from 'k6/http';

const targetRate = Number(__ENV.RATE || 100000);
const duration = __ENV.DURATION || '10s';
const targetUrl = __ENV.TARGET || 'http://127.0.0.1:8080/healthz';

export let options = {
  scenarios: {
    constant_rate: {
      executor: 'constant-arrival-rate',
      rate: targetRate,
      timeUnit: '1s',
      duration: duration,
      preAllocatedVUs: Math.max(1, Math.ceil(targetRate / 50)),
      maxVUs: Math.max(1000, Math.ceil(targetRate / 10)),
      gracefulStop: '30s',
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<500'],
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  http.get(targetUrl);
}
