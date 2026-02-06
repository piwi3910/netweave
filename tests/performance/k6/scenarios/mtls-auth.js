// mTLS Authentication Load Test
// Tests the performance of client certificate authentication
// SLA: p95 < 100ms, p99 < 500ms, error rate < 1%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { BASE_URL, API_VERSION, THRESHOLDS, SCENARIOS, MTLS_CERT, MTLS_KEY, CA_CERT } from '../config.js';
import { createMTLSParams, validateMTLSAuth } from '../helpers/auth.js';

// Custom metrics
const authLatency = new Trend('auth_latency', true);
const authErrors = new Rate('auth_errors');

// Test configuration
export const options = {
  scenarios: {
    mtls_load: SCENARIOS.load,
  },
  thresholds: THRESHOLDS.auth,
};

export default function () {
  // Create mTLS parameters
  const params = createMTLSParams(MTLS_CERT, MTLS_KEY, CA_CERT);
  params.tags = { scenario: 'auth' };

  // Test authenticated API call
  const startTime = Date.now();
  const res = http.get(`${BASE_URL}${API_VERSION}/resourcePools`, params);
  const duration = Date.now() - startTime;

  // Record metrics
  authLatency.add(duration);

  // Validate response
  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 100ms (p95 target)': (r) => r.timings.duration < 100,
    'response time < 500ms (p99 target)': (r) => r.timings.duration < 500,
    'mTLS certificate authenticated': (r) => validateMTLSAuth(r),
  });

  authErrors.add(!success);

  // Think time between requests (realistic user behavior)
  sleep(1);
}

// Test setup
export function setup() {
  console.log('Starting mTLS authentication load test');
  console.log(`Target: ${BASE_URL}${API_VERSION}`);
  console.log(`Certificate: ${MTLS_CERT}`);
  return {};
}

// Test teardown
export function teardown(data) {
  console.log('mTLS authentication load test completed');
}
