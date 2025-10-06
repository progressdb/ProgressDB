// test for claim 1k reqs/sec
export let options = {
  scenarios: {
    constant_rate: {
      executor: 'constant-arrival-rate',
      rate: 1000,       // requests per second
      timeUnit: '1s',
      // run short by default to send ~2k requests when iterating at 1k/sec
      duration: '1s',
      // pre-allocate enough VUs to sustain the target rate; adjust to match
      // expected latency (preAllocatedVUs ~= rate * expected_latency_s).
      preAllocatedVUs: 1000,
      maxVUs: 1200
    }
  },
  thresholds: {
    'http_req_duration': ['p(99)<100'],
    'http_req_failed': ['rate<0.01']
  }
};
