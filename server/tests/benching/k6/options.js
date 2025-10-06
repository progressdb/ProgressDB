export let options = {
  stages: [
    { duration: '10s', target: 200 },  // ramp up to 200 VUs
    { duration: '30s', target: 200 },  // hold 200 VUs
    { duration: '10s', target: 0 }     // ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(99)<100'], // 99% of requests must be under 100ms
    'http_req_failed': ['rate<0.01']    // less than 1% failures allowed
  }
};
