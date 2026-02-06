// Stress Test - Gradual Load Increase Until Failure
// Identifies breaking point and maximum capacity
// Gradually increases load until system shows degradation
// Monitors latency, error rates, and resource exhaustion

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { BASE_URL, API_VERSION, SCENARIOS, OAUTH2_TOKEN_URL, OAUTH2_CLIENT_ID, OAUTH2_CLIENT_SECRET, OAUTH2_USERNAME, OAUTH2_PASSWORD } from '../config.js';
import { getOAuth2Token, createOAuth2Headers } from '../helpers/auth.js';

// Custom metrics
const stressLatency = new Trend('stress_latency', true);
const stressErrors = new Rate('stress_errors');
const serverErrors = new Counter('server_errors_5xx');
const clientErrors = new Counter('client_errors_4xx');

// Test configuration - gradual stress increase
export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 50 },    // Stage 1: Warm up
        { duration: '3m', target: 50 },    // Stage 1: Baseline
        { duration: '2m', target: 100 },   // Stage 2: Double load
        { duration: '3m', target: 100 },   // Stage 2: Hold
        { duration: '2m', target: 200 },   // Stage 3: 2x load
        { duration: '3m', target: 200 },   // Stage 3: Hold
        { duration: '2m', target: 300 },   // Stage 4: 3x load
        { duration: '3m', target: 300 },   // Stage 4: Hold
        { duration: '2m', target: 400 },   // Stage 5: 4x load
        { duration: '3m', target: 400 },   // Stage 5: Hold
        { duration: '2m', target: 500 },   // Stage 6: 5x load
        { duration: '3m', target: 500 },   // Stage 6: Hold (looking for breaking point)
        { duration: '3m', target: 0 },     // Ramp down
      ],
    },
  },
  thresholds: {
    // We expect these to fail at some point - that's the goal
    'stress_errors': ['rate<0.1'], // < 10% errors during normal stages
    'http_req_duration': ['p(95)<500'], // Will likely fail at high load
  },
};

export function setup() {
  console.log('Setting up stress test...');
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

  console.log('Stress test ready');
  console.log('Scenario: Gradual increase 0 → 500 users');
  console.log('Goal: Find breaking point');
  return { token };
}

export default function (data) {
  const headers = createOAuth2Headers(data.token);
  const params = {
    headers,
    tags: { scenario: 'stress' },
  };

  // Mix of read and write operations
  const operations = [
    { method: 'GET', path: '/resourcePools', weight: 70 },
    { method: 'GET', path: '/resources', weight: 20 },
    { method: 'GET', path: '/subscriptions', weight: 8 },
    { method: 'POST', path: '/subscriptions', weight: 2, body: getTestSubscription() },
  ];

  // Weighted random selection
  const rand = Math.random() * 100;
  let cumWeight = 0;
  let selectedOp = operations[0];

  for (const op of operations) {
    cumWeight += op.weight;
    if (rand <= cumWeight) {
      selectedOp = op;
      break;
    }
  }

  const startTime = Date.now();
  let res;

  if (selectedOp.method === 'GET') {
    res = http.get(`${BASE_URL}${API_VERSION}${selectedOp.path}`, params);
  } else {
    params.headers['Content-Type'] = 'application/json';
    res = http.post(
      `${BASE_URL}${API_VERSION}${selectedOp.path}`,
      JSON.stringify(selectedOp.body),
      params
    );
  }

  const duration = Date.now() - startTime;
  stressLatency.add(duration);

  // Track different error types
  if (res.status >= 500) {
    serverErrors.add(1);
  } else if (res.status >= 400 && res.status < 500) {
    clientErrors.add(1);
  }

  // Validate response
  const success = check(res, {
    'request successful': (r) => r.status >= 200 && r.status < 300,
    'not server error': (r) => r.status < 500,
    'response received': (r) => r.body.length > 0,
  });

  stressErrors.add(!success);

  // Variable think time based on load
  const thinkTime = __VU <= 100 ? 1 : 0.5;
  sleep(thinkTime);
}

// Helper function to generate test subscription
function getTestSubscription() {
  return {
    callback: 'https://smo.example.com/notify',
    consumerSubscriptionId: `stress-test-${__VU}-${__ITER}`,
    filter: '(eq,resourceTypeId,node)',
  };
}

export function teardown(data) {
  console.log('Stress test completed');
  console.log('Analyze metrics to determine breaking point:');
  console.log('- When did p95 latency exceed 500ms?');
  console.log('- When did error rate exceed 5%?');
  console.log('- What was the maximum sustainable load?');
}
