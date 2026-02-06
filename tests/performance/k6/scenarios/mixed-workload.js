// Mixed Workload Load Test
// Tests 50/50 mix of mTLS and OAuth2 authentication
// Simulates realistic production traffic patterns
// SLA: p95 < 100ms, p99 < 500ms, error rate < 1%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { BASE_URL, API_VERSION, THRESHOLDS, SCENARIOS, MTLS_CERT, MTLS_KEY, CA_CERT, OAUTH2_TOKEN_URL, OAUTH2_CLIENT_ID, OAUTH2_CLIENT_SECRET, OAUTH2_USERNAME, OAUTH2_PASSWORD } from '../config.js';
import { getOAuth2Token, createOAuth2Headers, createMTLSParams, validateOAuth2Auth, validateMTLSAuth } from '../helpers/auth.js';

// Custom metrics
const mixedLatency = new Trend('mixed_workload_latency', true);
const mixedErrors = new Rate('mixed_workload_errors');
const mtlsRequests = new Counter('mtls_requests');
const oauth2Requests = new Counter('oauth2_requests');

// Test configuration
export const options = {
  scenarios: {
    mixed_load: SCENARIOS.load,
  },
  thresholds: {
    ...THRESHOLDS.api,
    'http_req_duration{auth_type:mtls}': ['p(95)<100', 'p(99)<500'],
    'http_req_duration{auth_type:oauth2}': ['p(95)<100', 'p(99)<500'],
  },
};

export function setup() {
  console.log('Setting up mixed workload test...');

  // Obtain OAuth2 token for the OAuth2 portion of traffic
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

  console.log('OAuth2 token obtained');
  console.log(`Target: ${BASE_URL}${API_VERSION}`);
  console.log('Mix: 50% mTLS, 50% OAuth2');

  return { token };
}

export default function (data) {
  // Randomly choose authentication method (50/50 split)
  const useMTLS = Math.random() < 0.5;

  let params;
  let authType;

  if (useMTLS) {
    // Use mTLS authentication
    params = createMTLSParams(MTLS_CERT, MTLS_KEY, CA_CERT);
    authType = 'mtls';
    mtlsRequests.add(1);
  } else {
    // Use OAuth2 authentication
    const headers = createOAuth2Headers(data.token);
    params = { headers };
    authType = 'oauth2';
    oauth2Requests.add(1);
  }

  params.tags = { auth_type: authType, scenario: 'api' };

  // Perform API operations with chosen auth method
  const operations = [
    { method: 'GET', path: '/resourcePools' },
    { method: 'GET', path: '/resources' },
    { method: 'GET', path: '/deploymentManagers' },
    { method: 'GET', path: '/subscriptions' },
  ];

  // Random operation
  const op = operations[Math.floor(Math.random() * operations.length)];

  const startTime = Date.now();
  const res = http.get(`${BASE_URL}${API_VERSION}${op.path}`, params);
  const duration = Date.now() - startTime;

  // Record metrics
  mixedLatency.add(duration);

  // Validate response
  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 100ms (p95)': (r) => r.timings.duration < 100,
    'response time < 500ms (p99)': (r) => r.timings.duration < 500,
    'authentication successful': (r) => {
      if (authType === 'mtls') {
        return validateMTLSAuth(r);
      }
      return validateOAuth2Auth(r);
    },
    'response has data': (r) => r.body.length > 0,
  });

  mixedErrors.add(!success);

  // Think time
  sleep(1);
}

export function teardown(data) {
  console.log('Mixed workload test completed');
}
