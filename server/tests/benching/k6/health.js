import http from 'k6/http';

// High-rate health endpoint benchmark. Adjust env var TARGET to point at the
// running server (default: http://127.0.0.1:8080/health).
export let options = {
  scenarios: {
    constant_rate: {
      executor: 'constant-arrival-rate',
      rate: 100000,       // target requests per second
      timeUnit: '1s',
      duration: '10s',    // short default run
      // pre-allocate enough VUs to sustain the target rate; tune to match
      // expected latency (preAllocatedVUs ~= rate * expected_latency_s).
      preAllocatedVUs: 2000,
      maxVUs: 4000
    }
  },
  thresholds: {
    'http_req_duration': ['p(99)<500'],
    'http_req_failed': ['rate<0.01']
  }
};

export default function () {
  const url = __ENV.TARGET || 'http://127.0.0.1:8080/health';
  http.get(url);
}
