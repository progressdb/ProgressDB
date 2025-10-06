// test for claim 1k reqs/sec
export let options = {
  scenarios: {
    constant_rate: {
      executor: 'constant-arrival-rate',
      rate: 1000,       // requests per second
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 600, // max VUs to handle the load
      maxVUs: 1000
    }
  },
  thresholds: {
    'http_req_duration': ['p(99)<100'],
    'http_req_failed': ['rate<0.01']
  }
};
