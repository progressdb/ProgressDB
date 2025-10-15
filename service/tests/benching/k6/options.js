// target: 4k reqs/sec for 2 minutes, strict SLOs
export let options = {
  scenarios: {
    constant_rate: {
      executor: 'constant-arrival-rate',
      rate: 3000,            // requests per second
      timeUnit: '1s',
      duration: '1m',        // 2 minutes to sustain pressure
      preAllocatedVUs: 2400, // enough for high concurrency & spike tolerance
      maxVUs: 3000
    }
  },
  thresholds: {
    'http_req_duration': [
      'p(95)<25',   // 95% of requests below 25ms
      'p(99)<50'    // 99% of requests below 50ms
    ],
    'http_req_failed': [
      'rate<0.005'  // less than 0.5% errors allowed
    ]
  }
};
