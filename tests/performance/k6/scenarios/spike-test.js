// Spike Test - Sudden Load Increase
// Tests system behavior under sudden traffic spikes (0 to 10k req/sec)
// Validates auto-scaling, rate limiting, and graceful degradation
// SLA: System should remain responsive, error rate should stay < 5% during spike

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { BASE_URL, API_VERSION, SCENARIOS, OAUTH2_TOKEN_URL, OAUTH2_CLIENT_ID, OAUTH2_CLIENT_SECRET, OAUTH2_USERNAME, OAUTH2_PASSWORD } from '../config.js';
import { getOAuth2Token, createOAuth2Headers } from '../helpers/auth.js';

// Custom metrics
const spikeLatency = new Trend('spike_latency', true);
const spikeErrors = new Rate('spike_errors');
const rateLimitHits = new Counter('rate_limit_hits');
const requestCount = new Counter('total_requests');

// Test configuration - aggressive spike scenario
export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 10 },     // Baseline: 10 users
        { duration: '10s', target: 1000 },   // Spike: 0→1000 users in 10s
        { duration: '2m', target: 1000 },    // Hold: maintain spike
        { duration: '10s', target: 10 },     // Recovery: back to baseline
        { duration: '1m', target: 10 },      // Stabilize
      ],
    },
  },
  thresholds: {
    'spike_errors': ['rate<0.05'], // Allow up to 5% error rate during spike
    'http_req_duration': ['p(95)<1000'], // Relaxed threshold during spike
    'http_req_duration{stage:baseline}': ['p(95)<100'], // Strict baseline
    'http_req_duration{stage:spike}': ['p(95)<1000'], // Relaxed during spike
  },
};

export function setup() {
  console.log('Setting up spike test...');
  const token = getOAuth2Token(
    OAUTH2_TOKEN_URL,
    OAUTH2_CLIENT_ID,
    OAUTH2_CLIENT_SECRET,
    OAUTH2_USERNAME,
    OAUTH2_PASSWORD
  );

  if (!token) {
    throw new Error('Failed to obtain OAuth2 access token');
  }

  console.log('Spike test ready');
  console.log('Scenario: 10 → 1000 users in 10 seconds');
  return { token };
}

export default function (data) {
  // Determine current stage based on __VU and __ITER
  const stage = __VU <= 10 ? 'baseline' : 'spike';

  const headers = createOAuth2Headers(data.token);
  const params = {
    headers,
    tags: { scenario: 'spike', stage },
  };

  requestCount.add(1);

  // Test various endpoints
  const endpoints = [
    '/resourcePools',
    '/resources',
    '/deploymentManagers',
    '/subscriptions',
  ];

  const endpoint = endpoints[Math.floor(Math.random() * endpoints.length)];

  const startTime = Date.now();
  const res = http.get(`${BASE_URL}${API_VERSION}${endpoint}`, params);
  const duration = Date.now() - startTime;

  spikeLatency.add(duration, { stage });

  // Check for rate limiting
  if (res.status === 429) {
    rateLimitHits.add(1);
  }

  // Validate response (allow 429 during spike)
  const success = check(res, {
    'not server error (5xx)': (r) => r.status < 500,
    'authenticated': (r) => r.status !== 401 && r.status !== 403,
    'response received': (r) => r.status === 200 || r.status === 429,
  });

  spikeErrors.add(!success);

  // Minimal think time during spike
  sleep(0.1);
}

export function teardown(data) {
  console.log('Spike test completed');
  console.log('Check metrics for rate limiting and error rates during spike');
}
